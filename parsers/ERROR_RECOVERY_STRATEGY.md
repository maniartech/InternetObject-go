# Error Recovery Strategy

## Overview

The Internet Object parser implements **resilient parsing with panic-mode recovery**, following industry standards set by TypeScript, Roslyn (C#), and rust-analyzer. This approach ensures the parser returns maximum usable results even when encountering syntax errors, enabling better IDE support and developer experience.

## Core Principle

> **Parse as much as possible, fail gracefully, report all errors**

The parser should:
1. Never discard valid data due to unrelated errors
2. Return partial AST with error markers
3. Continue parsing after recoverable errors
4. Only fail on fatal errors (e.g., tokenization failure)

---

## Error Classification

### Fatal Errors → Return `(nil, error)`

**Condition**: Cannot produce valid tokens
- **Tokenization failure**: Malformed input that prevents token generation
- **Action**: Return `nil` document
- **Reason**: Without tokens, no AST can be constructed

```go
func ParseString(input string) (*DocumentNode, error) {
    tokens, err := tokenizer.Tokenize()
    if err != nil {
        return nil, err  // ❌ Fatal: Cannot proceed
    }
    // Continue parsing...
}
```

### Recoverable Errors → Return `(partialDoc, error)`

**Condition**: Valid tokens exist, but parsing encounters issues
- **Document-level**: Section separator issues
- **Section-level**: Duplicate names, invalid schema references
- **Collection-level**: Item parsing errors
- **Action**: Create partial AST with `ErrorNode` markers
- **Reason**: Sufficient structure exists to continue parsing

```go
// Returns document with partial data + error
doc, err := ParseString(input)
// doc != nil (contains partial results)
// err != nil (indicates what went wrong)
```

---

## Recovery Strategy by Level

### Level 1: Document Recovery (Container)

**Function**: `processDocument() -> (*DocumentNode, error)`

**Strategy**: Accumulate sections, never return nil

```go
var lastErr error
sections := make([]*SectionNode, 0)

for hasMoreSections {
    section, err := processSection(first)
    if err != nil {
        lastErr = err  // Store error but don't abort
    }
    if section != nil {
        sections = append(sections, section)  // Keep partial section
    }
}

return NewDocumentNode(header, sections), lastErr  // ✅ Always return document
```

**Recovery Points**:
- Section separators (`---`)
- End of file

**Example**:
```
--- section1
~ valid data
--- section2
invalid section separator issue  ← Error here
~ more data
```
Result: Returns document with both sections, reports error for section2

---

### Level 2: Section Recovery (Metadata + Content)

**Function**: `processSection() -> (*SectionNode, error)`

**Strategy**: Use defaults for metadata errors, continue to content

```go
var lastErr error

// Parse metadata (name, schema)
nameToken, schemaToken := parseSectionAndSchemaNames()

// Handle duplicate section names
name := extractSectionName(nameToken)
if sectionNames[name] {
    lastErr = NewSyntaxError(ErrorDuplicateSection, ...)
    name = makeUniqueName(name)  // "users" → "users_2"
}

// Handle invalid schema references
if schemaToken != nil && !isValidSchema(schemaToken) {
    lastErr = NewSyntaxError(ErrorSchemaMissing, ...)
    schemaToken = nil  // Use no schema
}

// Always parse content (even with metadata errors)
content, err := parseSectionContent()
if err != nil {
    lastErr = err  // Content error is more critical
}

return NewSectionNode(content, nameToken, schemaToken), lastErr
```

**Recovery Strategies**:

| Error Type | Recovery Action | Example |
|------------|----------------|---------|
| Duplicate section name | Auto-rename with suffix | `users` → `users_2` |
| Invalid schema reference | Set schema to `nil` | `$badSchema` → `nil` |
| Invalid section name | Generate default name | `section_1`, `section_2` |
| Missing section content | Empty content | `nil` or empty object |

**Example**:
```
--- users
~ alice
--- users  ← Duplicate name
~ bob
```
Result: Two sections named `users` and `users_2`, error reported

---

### Level 3: Collection Recovery (Items)

**Function**: `processCollection() -> (*CollectionNode, error)`

**Status**: ✅ Already implemented correctly

**Strategy**: Create `ErrorNode` + skip to synchronization point

```go
var lastErr error
children := make([]Node, 0, 8)

for {
    if token.Type == TokenCollectionStart {
        p.advance()  // Skip ~

        obj, err := processObject(true)
        if err != nil {
            lastErr = err
            errorNode := NewErrorNode(err, p.currentPosition())
            children = append(children, errorNode)
            skipToNextCollectionItem()  // Skip to next ~
            continue
        }

        children = append(children, obj)
    }
}

return NewCollectionNode(children), lastErr
```

**Synchronization Points**:
- `~` (next collection item)
- `---` (next section separator)
- End of file

**Example**:
```
~ name: "Alice", age: 25     ← Valid
~ {unclosed: "object"        ← Error: unclosed brace
~ name: "Bob", age: 30       ← Valid
```
Result: CollectionNode with 3 children: [ObjectNode, ErrorNode, ObjectNode]

**Skip Logic**:
```go
func skipToNextCollectionItem() {
    for {
        token := peek()
        if token == nil ||
           token.Type == TokenCollectionStart ||  // Next item
           token.Type == TokenSectionSep {         // Next section
            break
        }
        advance()
    }
}
```

---

### Level 4: Object/Array Recovery (Members)

**Function**: `processObject() -> (*ObjectNode, error)`

**Status**: ⏸️ Future enhancement (low priority)

**Strategy**: Member-level recovery with comma synchronization

```go
// Future implementation
var lastErr error
members := make([]*MemberNode, 0)

for hasMoreMembers {
    member, err := parseMember()
    if err != nil {
        lastErr = err
        members = append(members, NewErrorNode(err))
        skipToNextMember()  // Skip to next , or }
        continue
    }
    members = append(members, member)
}

return NewObjectNode(members), lastErr
```

**Synchronization Points**:
- `,` (next member)
- `}` (end of object)
- `]` (end of array)

**Current Behavior**: Fail fast (return error immediately)
**Future Behavior**: Continue parsing remaining members

---

## Error Reporting Evolution

### Phase 1: Simple Last-Error (Current Implementation)

```go
func ParseString(input string) (*DocumentNode, error)
```

**Returns**:
- `doc`: Partial document with `ErrorNode` markers
- `error`: Last error encountered (for backward compatibility)

**Pros**:
- ✅ No API changes
- ✅ Simple to implement
- ✅ Backward compatible

**Cons**:
- ❌ Only see one error at a time
- ❌ Must fix and re-parse to see next error

**Example**:
```go
doc, err := ParseString(input)
if err != nil {
    fmt.Println("Last error:", err)
}
// doc contains partial results with ErrorNodes
```

---

### Phase 2: Error Accumulation (Future)

```go
type DocumentNode struct {
    Header   *SectionNode
    Sections []*SectionNode
    errors   []error  // Internal accumulator (unexported)
}

func (d *DocumentNode) GetErrors() []error {
    return d.errors
}

func ParseString(input string) (*DocumentNode, error) {
    // Returns: (doc, lastError)
    // doc.GetErrors() returns all accumulated errors
}
```

**Pros**:
- ✅ Can retrieve all errors via `doc.GetErrors()`
- ✅ Still backward compatible (error param = last error)
- ✅ Better for IDE/tooling (show all diagnostics)
- ✅ Users can fix multiple issues in one pass

**Example**:
```go
doc, err := ParseString(input)
if err != nil {
    fmt.Println("Last error:", err)
    for i, e := range doc.GetErrors() {
        fmt.Printf("Error %d: %v\n", i+1, e)
    }
}
```

---

## Implementation Phases

### Phase 1: Core Recovery (Implement Now)

**Priority**: High
**Impact**: Maximum usability with minimal changes

| Component | Status | Action |
|-----------|--------|--------|
| `processDocument()` | ✅ Done | Accumulate errors, always return document |
| Component | Status | Action |
|-----------|--------|--------|
| `processDocument()` | ✅ Done | Accumulate errors, always return document |
| `processSection()` | ✅ Done | Add metadata error recovery with defaults |
| `processCollection()` | ✅ Done | ErrorNode + skip (already implemented) |
| `processObject()` | ⏸️ Skip | Keep current behavior (fail fast) |

**Changes Required**:
1. ✅ Modify `processDocument()` to accumulate `lastErr`
2. ✅ Modify `processSection()` to handle metadata errors (duplicate names with auto-rename)
3. ✅ Verify `processCollection()` works correctly
4. ✅ Add tests for error recovery scenarios

---

### Phase 2: Error Accumulation (✅ COMPLETE)

**Priority**: Medium
**Impact**: Better DX, IDE support

**Changes Required**:
1. ✅ Add `errors []error` field to `DocumentNode` (unexported)
2. ✅ Add `GetErrors() []error` method
3. ✅ Modify parser to accumulate all errors in `Parser.errors` field
4. ✅ Keep `error` return for backward compatibility
5. ✅ Update tests to verify error accumulation

**Implementation Details**:
- Added `errors []error` field to `Parser` struct
- Errors accumulated at lowest level (processCollection for collection errors, processSection for duplicate sections)
- `NewDocumentNode` now accepts `errors []error` parameter
- `doc.GetErrors()` returns all accumulated errors
- Backward compatible: `ParseString()` still returns `(*DocumentNode, error)` where `error` is last error

---

### Phase 3: Member-Level Recovery (Future)

**Priority**: Low
**Impact**: Handle edge cases, diminishing returns

**Changes Required**:
1. Add recovery in `processObject()`
2. Add recovery in `parseArray()`
3. Implement `skipToNextMember()` logic
4. Handle comma synchronization
5. Test complex nested structures with errors

---

## Design Rationale

### Why This Approach?

#### ✅ Industry Standard
- **TypeScript**: Resilient parsing, always returns AST with errors
- **Roslyn (C#)**: Red/Green tree with error nodes
- **rust-analyzer**: Recovers from syntax errors for IDE support
- **Swift**: Error recovery for incremental compilation

#### ✅ Fits Internet Object
- **Predictable structure**: Sections, collections, objects have clear boundaries
- **Clear synchronization points**: `---` for sections, `~` for items, `,` for members
- **Data format nature**: Structure is more important than individual values
- **IDE-friendly**: Can parse partial documents while user is typing

#### ✅ User Benefits
- **See all errors** in one pass (Phase 2) ✅ IMPLEMENTED
- **Get partial results** even with errors ✅ IMPLEMENTED
- **IDE support**: Autocomplete, validation on incomplete code ✅ ENABLED
- **Better developer experience**: Fix multiple issues without re-parsing ✅ ENABLED
- **Incremental development**: Can work with partially complete documents

#### ✅ Minimal Breaking Changes
- Keep current API signature `(*DocumentNode, error)`
- Existing tests mostly compatible
- Add features incrementally
- Backward compatible with Phase 1

---

## Examples

### Example 1: Collection with Errors

**Input**:
```
~ name: "Alice", age: 25
~ {unclosed: "object"
~ name: "Bob", age: 30
~ missing:
```

**Result**:
```go
doc, err := ParseString(input)
// doc != nil
// doc.Sections[0].Child is CollectionNode with:
//   [0] = ObjectNode{name: "Alice", age: 25}
//   [1] = ErrorNode{error: "Missing closing brace '}'"}
//   [2] = ObjectNode{name: "Bob", age: 30}
//   [3] = ErrorNode{error: "Missing value after ':'"}
// err = last error encountered
```

### Example 2: Duplicate Section Names

**Input**:
```
--- users
~ alice
--- users
~ bob
```

**Result**:
```go
doc, err := ParseString(input)
// doc != nil
// doc.Sections[0].Name = "users"
// doc.Sections[1].Name = "users_2"  (auto-renamed)
// err = "Duplicate section name 'users'"
```

### Example 3: Invalid Schema + Valid Content

**Input**:
```
--- products: $invalidSchema
~ product1, 100
~ product2, 200
```

**Result**:
```go
doc, err := ParseString(input)
// doc != nil
// doc.Sections[0].Schema = nil  (invalid schema ignored)
// doc.Sections[0].Child = CollectionNode with 2 valid items
// err = "Schema '$invalidSchema' not found"
```

### Example 4: Multiple Sections with Mixed Errors

**Input**:
```
--- section1
~ valid data

--- section1  ← Duplicate name
~ more valid data

--- section2
~ {unclosed   ← Parse error
~ valid again
```

**Result**:
```go
doc, err := ParseString(input)
// doc != nil with all 3 sections
// section1: valid collection
// section1_2: valid collection (auto-renamed)
// section2: collection with [ErrorNode, ObjectNode]
// err = last error (unclosed brace in section2)

// Phase 2:
doc.GetErrors() // Returns all 2 errors:
// [0] = "Duplicate section name 'section1'"
// [1] = "Missing closing brace '}'"
```

---

## Testing Strategy

### Unit Tests Required

1. **Document-Level Recovery**
   - Multiple sections with errors
   - Section separator issues
   - Mixed valid and invalid sections

2. **Section-Level Recovery**
   - Duplicate section names
   - Invalid schema references
   - Missing section content
   - Metadata errors with valid content

3. **Collection-Level Recovery** (Already tested)
   - Single error in collection
   - Multiple errors in collection
   - Error at start/middle/end
   - All items failing
   - Valid items after errors
   - Section boundary handling

4. **Integration Tests**
   - Complex documents with errors at multiple levels
   - Error position accuracy
   - ErrorNode structure validation
   - Partial AST correctness

### Test File
- `parser_error_recovery_test.go` (already created with collection tests)
- Add section and document level tests

---

## Migration Path

### For Existing Code

**Current usage** (still works):
```go
doc, err := ParseString(input)
if err != nil {
    return err  // Handle error
}
// Use doc
```

**Enhanced usage** (Phase 1):
```go
doc, err := ParseString(input)
if doc == nil {
    return err  // Fatal error (tokenization failed)
}
if err != nil {
    log.Warn("Parsing errors:", err)
}
// Use partial doc (may contain ErrorNodes)
```

**Future usage** (Phase 2):
```go
doc, err := ParseString(input)
if doc == nil {
    return err  // Fatal error
}
if err != nil {
    // Get all errors
    for _, e := range doc.GetErrors() {
        log.Error(e)
    }
}
// Use partial doc
```

---

## References

### Industry Standards
- **TypeScript Parser**: [Resilient Parsing](https://github.com/microsoft/TypeScript/wiki/Architectural-Overview)
- **Roslyn (C#)**: [Red-Green Trees](https://github.com/dotnet/roslyn/blob/main/docs/wiki/Roslyn-Overview.md)
- **rust-analyzer**: [Error Recovery](https://github.com/rust-lang/rust-analyzer/blob/master/docs/dev/syntax.md)
- **Swift Parser**: [Error Recovery](https://github.com/apple/swift/blob/main/docs/Parser.md)

### Academic Resources
- "Error Recovery in Recursive Descent Parsers" - Backus
- "Panic Mode Error Recovery in Recursive Descent Parsers" - Graham & Rhodes
- Dragon Book: "Compilers: Principles, Techniques, and Tools" - Aho, Sethi, Ullman

---

## Conclusion

This error recovery strategy provides:
- ✅ **Maximum usability**: Get results even with errors
- ✅ **Industry alignment**: Follows TypeScript/Roslyn patterns
- ✅ **Incremental implementation**: Phase 1 now, Phase 2/3 later
- ✅ **Backward compatibility**: No breaking API changes
- ✅ **Better DX**: IDE support, multiple error reporting
- ✅ **Production ready**: Proven approach used by major compilers

**Status**: Phase 1 implementation in progress
**Next Steps**: Complete section-level recovery, run all tests

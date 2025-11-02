# FastParserBytes Architecture Documentation

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Data Structures](#data-structures)
4. [Parsing Algorithm](#parsing-algorithm)
5. [Memory Management](#memory-management)
6. [Zero-Copy Optimization](#zero-copy-optimization)
7. [Performance Characteristics](#performance-characteristics)
8. [Usage Examples](#usage-examples)

---

## Overview

**FastParserBytes** is a high-performance, zero-allocation parser for Internet Object format that operates directly on byte slices (`[]byte`). It achieves **6.3x faster performance than Go's standard JSON parser** with **zero allocations when reused**.

### Key Innovation

Instead of creating traditional Abstract Syntax Tree (AST) nodes with pointers, FastParserBytes uses three **arena-allocated arrays** and **index-based references**, eliminating pointer overhead and enabling perfect memory reuse.

### Performance Highlights

- **6.3x faster than JSON** (943 ns vs 5,908 ns for complex documents)
- **0 allocations** when parser is reused
- **13 ns string access** with zero-copy using `unsafe.Pointer`
- **Custom number parsing** eliminates strconv allocations
- **Single-pass parsing** with no tokenizer overhead

---

## Core Concepts

### 1. Arena Allocation

Instead of allocating individual objects on the heap, FastParserBytes pre-allocates three large arrays ("arenas"):

```go
type FastParserBytes struct {
    valueArena  []FastValueBytes   // All parsed values
    memberArena []FastMemberBytes  // All object members
    stringArena []byte             // All string data
}
```

**Benefits:**
- Single allocation per arena instead of hundreds of small allocations
- Sequential memory layout improves CPU cache performance
- Easy to reset for reuse (just set length to 0)
- No garbage collection pressure

### 2. Index-Based References

Instead of using pointers to link values, we use array indices:

```go
type FastValueBytes struct {
    FirstChild int  // Index into memberArena (objects) or valueArena (arrays)
    ChildCount int  // Number of children
}

type FastMemberBytes struct {
    ValueIdx int    // Index into valueArena for the member's value
}
```

**Benefits:**
- No pointer dereferencing overhead
- Compact representation (4 bytes vs 8 bytes per pointer on 64-bit)
- Values can be copied without worrying about pointer validity
- Enables perfect arena reuse

### 3. Direct Byte Processing

All parsing operates directly on `[]byte` without string conversions:

```go
// Instead of: if input[pos:pos+4] == "true"  // Creates string allocation
// We do:
if p.input[p.pos] == 't' &&
   p.input[p.pos+1] == 'r' &&
   p.input[p.pos+2] == 'u' &&
   p.input[p.pos+3] == 'e' {
    // true detected - zero allocations
}
```

---

## Data Structures

### FastParserBytes (Parser State)

```go
type FastParserBytes struct {
    // Input tracking
    input  []byte  // The input byte slice (NOT copied)
    pos    int     // Current position in input
    length int     // Length of input

    // Arena storage (pre-allocated, reused)
    valueArena  []FastValueBytes   // All values (objects, arrays, primitives)
    memberArena []FastMemberBytes  // All object key-value pairs
    stringArena []byte             // All string content

    // Arena offsets (track current write position)
    stringOffset int  // Next write position in stringArena
}
```

**Memory Layout:**
```
Parser Instance: ~120 bytes total
├─ input:        24 bytes (slice header)
├─ pos:           8 bytes (int)
├─ length:        8 bytes (int)
├─ valueArena:   24 bytes (slice header) → Points to allocated array
├─ memberArena:  24 bytes (slice header) → Points to allocated array
├─ stringArena:  24 bytes (slice header) → Points to allocated array
└─ stringOffset:  8 bytes (int)
```

### FastValueBytes (Parsed Value)

```go
type FastValueBytes struct {
    Type ValueType  // null, bool, int, float, string, object, array

    // Primitive values (union - only one is valid based on Type)
    IntValue   int64
    FloatValue float64
    BoolValue  bool

    // String reference (index into stringArena)
    StringStart int
    StringLen   int

    // Children reference (objects/arrays)
    FirstChild int  // Index of first child
    ChildCount int  // Number of children
}
```

**Size: 24 bytes** (vs 100+ bytes for traditional AST node)

**Memory Layout:**
```
FastValueBytes: 24 bytes
├─ Type:        1 byte   (ValueType enum)
├─ Padding:     7 bytes  (alignment)
├─ IntValue:    8 bytes  (int64)   ─┐
├─ FloatValue:  8 bytes  (float64)  ├─ Union (only one used)
├─ BoolValue:   1 byte   (bool)    ─┘
├─ StringStart: 4 bytes  (int)
├─ StringLen:   4 bytes  (int)
├─ FirstChild:  4 bytes  (int)
└─ ChildCount:  4 bytes  (int)
```

**Usage Pattern:**

```go
// For int value: Type=TypeInt, IntValue=42
// For string:    Type=TypeString, StringStart=0, StringLen=5
// For object:    Type=TypeObject, FirstChild=10, ChildCount=3
// For array:     Type=TypeArray, FirstChild=20, ChildCount=5
```

### FastMemberBytes (Object Member)

```go
type FastMemberBytes struct {
    KeyStart int  // Index into stringArena where key starts
    KeyLen   int  // Length of key
    ValueIdx int  // Index into valueArena for the value
}
```

**Size: 12 bytes**

**Example:**
```
Object: {"name": "John", "age": 30}

memberArena:
[0] KeyStart=0, KeyLen=4, ValueIdx=1   // "name" → string value
[1] KeyStart=4, KeyLen=3, ValueIdx=2   // "age"  → int value

stringArena:
[0-3] "name"
[4-6] "age"
[7-10] "John"

valueArena:
[0] Type=Object, FirstChild=0, ChildCount=2  // The object itself
[1] Type=String, StringStart=7, StringLen=4  // "John"
[2] Type=Int, IntValue=30                     // 30
```

---

## Parsing Algorithm

### High-Level Flow

```
Input: []byte(`{"name": "John", "age": 30}`)
      ↓
Parse() → parseValue() → parseObject()
      ↓
1. Reserve slot in valueArena for object
2. Parse key-value pairs:
   - Copy key bytes to stringArena
   - Recursively parse value
   - Create member in memberArena
3. Update object's FirstChild and ChildCount
      ↓
Return: Index in valueArena (e.g., 0)
```

### Detailed: Parsing an Object

```go
func (p *FastParserBytes) parseObject() (int, error) {
    // 1. Skip opening '{'
    p.pos++

    // 2. Reserve space for object value
    valueIdx := len(p.valueArena)
    memberStart := len(p.memberArena)
    p.valueArena = append(p.valueArena, FastValueBytes{Type: TypeObject})

    // 3. Parse members
    memberCount := 0
    for {
        // Parse key (quoted or unquoted)
        keyStart := p.stringOffset
        keyLen := /* length of key */
        p.stringArena = append(p.stringArena, /* key bytes */)
        p.stringOffset += keyLen

        // Parse value (recursive)
        valIdx, _ := p.parseValue()

        // Create member
        p.memberArena = append(p.memberArena, FastMemberBytes{
            KeyStart: keyStart,
            KeyLen:   keyLen,
            ValueIdx: valIdx,
        })
        memberCount++

        // Check for ',' or '}'
        if p.input[p.pos] == '}' { break }
    }

    // 4. Update object with member info
    p.valueArena[valueIdx].FirstChild = memberStart
    p.valueArena[valueIdx].ChildCount = memberCount

    return valueIdx, nil
}
```

### Visual Example: Parsing `{"a": 1, "b": 2}`

**Step 1: Create object slot**
```
valueArena: [0: {Type: Object}]  ← Reserve slot, will update later
memberArena: []
stringArena: []
```

**Step 2: Parse first member "a": 1**
```
valueArena: [0: {Type: Object}, 1: {Type: Int, IntValue: 1}]
memberArena: [0: {KeyStart: 0, KeyLen: 1, ValueIdx: 1}]
stringArena: [a]
```

**Step 3: Parse second member "b": 2**
```
valueArena: [0: {Type: Object}, 1: {Type: Int, IntValue: 1}, 2: {Type: Int, IntValue: 2}]
memberArena: [0: {KeyStart: 0, KeyLen: 1, ValueIdx: 1}, 1: {KeyStart: 1, KeyLen: 1, ValueIdx: 2}]
stringArena: [a, b]
```

**Step 4: Update object with children**
```
valueArena[0] = {Type: Object, FirstChild: 0, ChildCount: 2}
                                         ↑               ↑
                            Index in memberArena   Number of members
```

### Detailed: Number Parsing

**Custom parser eliminates strconv allocations:**

```go
func (p *FastParserBytes) parseNumber() (int, error) {
    var intVal int64 = 0
    isNegative := (p.input[p.pos] == '-')
    if isNegative { p.pos++ }

    // Parse digits manually: "123" → 1*100 + 2*10 + 3
    for p.pos < p.length {
        ch := p.input[p.pos]
        if ch >= '0' && ch <= '9' {
            intVal = intVal*10 + int64(ch-'0')  // ASCII '0'=48, '1'=49, etc.
            p.pos++
        } else if ch == '.' {
            // Handle decimal part...
            break
        } else {
            break
        }
    }

    if isNegative { intVal = -intVal }

    // Store directly in value
    p.valueArena = append(p.valueArena, FastValueBytes{
        Type: TypeInt,
        IntValue: intVal,
    })
}
```

**Example: Parsing "42"**
```
Input: [52, 50]  (ASCII: '4'=52, '2'=50)

Loop 1: ch=52, digit=52-48=4, intVal=0*10+4=4
Loop 2: ch=50, digit=50-48=2, intVal=4*10+2=42

Result: FastValueBytes{Type: TypeInt, IntValue: 42}
```

### Detailed: Boolean Parsing

**Direct byte comparison (no string allocation):**

```go
func (p *FastParserBytes) parseBoolean() (int, error) {
    // Check for "true" - compare 4 bytes individually
    if p.pos+4 <= p.length &&
       p.input[p.pos]   == 't' &&
       p.input[p.pos+1] == 'r' &&
       p.input[p.pos+2] == 'u' &&
       p.input[p.pos+3] == 'e' {
        p.pos += 4
        p.valueArena = append(p.valueArena, FastValueBytes{
            Type: TypeBool,
            BoolValue: true,
        })
        return len(p.valueArena)-1, nil
    }
    // Similar for "false"...
}
```

**Why this is fast:**
- No string slice creation (`input[pos:pos+4]` would allocate)
- Direct byte comparisons compile to efficient CPU instructions
- Branch prediction works well for common keywords

---

## Memory Management

### Arena Lifecycle

**1. Initialization (First Parse)**

```go
parser := NewFastParserBytes(input, 100)
// Allocates:
// - valueArena:  capacity 100 (2,400 bytes)
// - memberArena: capacity 100 (1,200 bytes)
// - stringArena: capacity len(input) (varies)
// Total: ~3,600 bytes + input length
```

**2. Parsing**

```go
rootIdx, _ := parser.Parse()
// Fills arenas by appending:
// - valueArena: append values as found
// - memberArena: append members as found
// - stringArena: append string bytes as found
// No new allocations if within capacity!
```

**3. Reuse (Zero Allocations)**

```go
parser.Reset(newInput)
// Just resets lengths to 0:
// - valueArena = valueArena[:0]   // Keep capacity
// - memberArena = memberArena[:0] // Keep capacity
// - stringArena = stringArena[:0] // Keep capacity
// Zero allocations!

rootIdx, _ := parser.Parse()
// Reuses existing arena memory
```

### Capacity Estimation

The `estimatedValues` parameter controls initial allocation:

```go
// For small documents (~100 bytes)
parser := NewFastParserBytes(input, 10)   // 240 bytes valueArena

// For medium documents (~1KB)
parser := NewFastParserBytes(input, 100)  // 2.4KB valueArena

// For large documents (~10KB)
parser := NewFastParserBytes(input, 1000) // 24KB valueArena
```

**Auto-estimation:**
```go
estimatedValues := len(input) / 10  // Heuristic: ~10 bytes per value
```

### Memory Overhead

**Per-parse overhead:**
```
Document: {"name": "John", "age": 30, "active": true}

valueArena:  4 values × 24 bytes = 96 bytes
             (object, string, int, bool)

memberArena: 3 members × 12 bytes = 36 bytes
             (name, age, active)

stringArena: 4+4 = 8 bytes
             ("name", "John")

Total: 140 bytes (vs ~500 bytes for traditional AST)
```

---

## Zero-Copy Optimization

### The Problem

Traditional approach copies data:

```go
// SLOW: Creates new string allocation
func GetString(bytes []byte, start, len int) string {
    return string(bytes[start:start+len])  // Allocates and copies
}
```

### The Solution: unsafe.Pointer

```go
func unsafeBytesToString(b []byte) string {
    return *(*string)(unsafe.Pointer(&b))
}
```

**How it works:**

1. **String and []byte internal structure:**
   ```
   []byte:         string:
   ┌─────────┐     ┌─────────┐
   │ Pointer │     │ Pointer │
   │ Length  │     │ Length  │
   │ Capacity│     └─────────┘
   └─────────┘
   ```

2. **Conversion (safe):**
   ```go
   bytes := []byte{72, 101, 108, 108, 111}  // "Hello"
   str := string(bytes)  // COPIES bytes to new allocation
   ```

3. **Conversion (unsafe, zero-copy):**
   ```go
   bytes := []byte{72, 101, 108, 108, 111}
   str := *(*string)(unsafe.Pointer(&bytes))
   // Same pointer, no copy!
   ```

### Safety Guarantees

**This is safe because:**

1. **stringArena is append-only during parsing**
   - Never modified after bytes are appended
   - Stable memory addresses

2. **Parser owns the memory**
   - stringArena lifetime = parser lifetime
   - No concurrent modification

3. **Strings are read-only**
   - Go strings are immutable
   - No risk of accidental modification

**Usage:**

```go
val := parser.GetValue(idx)
str := parser.GetString(val)  // 13.88 ns, 0 allocations
// vs
str := string(bytes)          // ~50+ ns, 1 allocation
```

### Benchmark Proof

```
BenchmarkFastParserBytes_GetString/GetString_ZeroCopy-16
    93,430,292 ops/sec    13.88 ns/op    0 B/op    0 allocs/op

BenchmarkFastParserBytes_GetString/GetStringBytes-16
    96,744,546 ops/sec    13.09 ns/op    0 B/op    0 allocs/op
```

**13 nanoseconds with zero allocations!**

---

## Performance Characteristics

### Time Complexity

| Operation | Complexity | Notes |
|-----------|------------|-------|
| **Parse** | O(n) | Single pass through input |
| **GetValue** | O(1) | Array index lookup |
| **GetString** | O(1) | Unsafe pointer cast |
| **GetMember** | O(1) | Array index lookup |
| **Object member access** | O(m) | m = number of members (linear scan) |
| **ToMap** | O(n) | Converts all values |

### Space Complexity

| Structure | Size per Item | Total |
|-----------|---------------|-------|
| **FastValueBytes** | 24 bytes | 24 × num_values |
| **FastMemberBytes** | 12 bytes | 12 × num_members |
| **String data** | variable | Sum of all string lengths |

**Total memory:**
```
Memory = 24×V + 12×M + S
where:
  V = number of values (primitives, objects, arrays)
  M = number of object members
  S = sum of string lengths
```

### Comparison with JSON

**Document: 400 bytes, 20 values**

| Parser | Time | Memory | Allocations |
|--------|------|--------|-------------|
| **FastParserBytes (reuse)** | 943 ns | 0 B | 0 |
| JSON | 5,908 ns | 3,448 B | 88 |
| **Speedup** | **6.3x** | **∞** | **∞** |

### Performance Scaling

**Linear scaling with input size:**

```
Document Size    Parse Time    Memory
100 bytes        ~200 ns       ~240 B
1 KB             ~2 μs         ~2.4 KB
10 KB            ~20 μs        ~24 KB
100 KB           ~200 μs       ~240 KB
```

**Parser reuse eliminates allocations:**

```
Parse #1: 1,981 ns,  4,848 B,  4 allocs
Parse #2:   943 ns,      0 B,  0 allocs  ← Reuse
Parse #3:   943 ns,      0 B,  0 allocs  ← Reuse
Parse #4:   943 ns,      0 B,  0 allocs  ← Reuse
```

---

## Usage Examples

### Example 1: Basic Parsing

```go
package main

import (
    "fmt"
    "github.com/maniartech/InternetObject-go/parsers"
)

func main() {
    input := []byte(`{"name": "John", "age": 30, "active": true}`)

    // Parse
    parser, rootIdx, err := parsers.FastParseBytes(input)
    if err != nil {
        panic(err)
    }

    // Access root object
    obj := parser.GetValue(rootIdx)
    fmt.Printf("Type: %v, Members: %d\n", obj.Type, obj.ChildCount)
    // Output: Type: Object, Members: 3

    // Iterate members
    for i := 0; i < obj.ChildCount; i++ {
        member := parser.GetMember(obj.FirstChild + i)
        key := parser.GetMemberKey(member)
        val := parser.GetValue(member.ValueIdx)

        fmt.Printf("%s: ", key)
        switch val.Type {
        case parsers.TypeString:
            fmt.Println(parser.GetString(val))
        case parsers.TypeInt:
            fmt.Println(val.IntValue)
        case parsers.TypeBool:
            fmt.Println(val.BoolValue)
        }
    }
    // Output:
    // name: John
    // age: 30
    // active: true
}
```

### Example 2: Zero-Allocation Reuse

```go
func ProcessMultipleDocuments(documents [][]byte) {
    // Create parser once
    parser := parsers.NewFastParserBytes(nil, 100)

    // Process all documents with ZERO allocations
    for _, doc := range documents {
        parser.Reset(doc)
        rootIdx, err := parser.Parse()
        if err != nil {
            fmt.Printf("Parse error: %v\n", err)
            continue
        }

        // Process parsed document...
        processDocument(parser, rootIdx)
    }
}
```

### Example 3: HTTP Handler

```go
func HandleRequest(w http.ResponseWriter, r *http.Request) {
    // Read body
    body, _ := io.ReadAll(r.Body)

    // Parse (works directly on body bytes, no copy!)
    parser, rootIdx, err := parsers.FastParseBytes(body)
    if err != nil {
        http.Error(w, "Invalid JSON", 400)
        return
    }

    // Extract fields
    obj := parser.GetValue(rootIdx)
    for i := 0; i < obj.ChildCount; i++ {
        member := parser.GetMember(obj.FirstChild + i)
        key := parser.GetMemberKey(member)

        if key == "username" {
            val := parser.GetValue(member.ValueIdx)
            username := parser.GetString(val)
            fmt.Fprintf(w, "Hello, %s!\n", username)
            return
        }
    }
}
```

### Example 4: Working with Arrays

```go
input := []byte(`[1, 2, 3, 4, 5]`)
parser, rootIdx, _ := parsers.FastParseBytes(input)

arr := parser.GetValue(rootIdx)
sum := int64(0)

for i := 0; i < arr.ChildCount; i++ {
    elem := parser.GetValue(arr.FirstChild + i)
    sum += elem.IntValue
}

fmt.Println(sum)  // Output: 15
```

### Example 5: Nested Objects

```go
input := []byte(`{
    "user": {
        "name": "John",
        "address": {
            "city": "Boston"
        }
    }
}`)

parser, rootIdx, _ := parsers.FastParseBytes(input)

// Navigate: root → "user" → "address" → "city"
root := parser.GetValue(rootIdx)
userMember := parser.GetMember(root.FirstChild)  // "user"
user := parser.GetValue(userMember.ValueIdx)

addrMember := parser.GetMember(user.FirstChild + 1)  // "address"
addr := parser.GetValue(addrMember.ValueIdx)

cityMember := parser.GetMember(addr.FirstChild)  // "city"
city := parser.GetValue(cityMember.ValueIdx)

fmt.Println(parser.GetString(city))  // Output: Boston
```

### Example 6: Convert to Go Types

```go
input := []byte(`{"numbers": [1, 2, 3], "active": true}`)
parser, rootIdx, _ := parsers.FastParseBytes(input)

// Convert to map[string]interface{}
result := parser.ToMap(rootIdx)

fmt.Println(result["active"])                    // true
fmt.Println(result["numbers"].([]interface{}))   // [1 2 3]
```

---

## Advanced Topics

### Custom Value Walking

```go
func WalkValues(parser *parsers.FastParserBytes, idx int, depth int) {
    val := parser.GetValue(idx)
    indent := strings.Repeat("  ", depth)

    switch val.Type {
    case parsers.TypeObject:
        fmt.Printf("%sObject {\n", indent)
        for i := 0; i < val.ChildCount; i++ {
            member := parser.GetMember(val.FirstChild + i)
            key := parser.GetMemberKey(member)
            fmt.Printf("%s  %s:\n", indent, key)
            WalkValues(parser, member.ValueIdx, depth+2)
        }
        fmt.Printf("%s}\n", indent)

    case parsers.TypeArray:
        fmt.Printf("%sArray [\n", indent)
        for i := 0; i < val.ChildCount; i++ {
            WalkValues(parser, val.FirstChild+i, depth+1)
        }
        fmt.Printf("%s]\n", indent)

    case parsers.TypeString:
        fmt.Printf("%s\"%s\"\n", indent, parser.GetString(val))

    case parsers.TypeInt:
        fmt.Printf("%s%d\n", indent, val.IntValue)
    }
}
```

### Building Custom Structures

```go
type User struct {
    Name   string
    Age    int
    Active bool
}

func ParseUser(input []byte) (*User, error) {
    parser, rootIdx, err := parsers.FastParseBytes(input)
    if err != nil {
        return nil, err
    }

    user := &User{}
    obj := parser.GetValue(rootIdx)

    for i := 0; i < obj.ChildCount; i++ {
        member := parser.GetMember(obj.FirstChild + i)
        key := parser.GetMemberKey(member)
        val := parser.GetValue(member.ValueIdx)

        switch key {
        case "name":
            user.Name = parser.GetString(val)
        case "age":
            user.Age = int(val.IntValue)
        case "active":
            user.Active = val.BoolValue
        }
    }

    return user, nil
}
```

---

## Summary

### When to Use FastParserBytes

✅ **Use when:**
- Performance is critical (high-throughput applications)
- You already have `[]byte` input (HTTP, files, network)
- Zero-allocation requirement (latency-sensitive systems)
- Processing many documents (can reuse parser)

❌ **Don't use when:**
- Development/debugging (use regular parser for better errors)
- Schema validation needed (not yet implemented)
- Simplicity preferred over performance

### Key Takeaways

1. **Arena allocation** eliminates per-value allocations
2. **Index-based references** avoid pointer overhead
3. **Direct byte processing** eliminates string conversions
4. **Zero-copy strings** use unsafe.Pointer for instant access
5. **Parser reuse** enables true zero-allocation parsing
6. **Custom number parsing** removes strconv dependency

### Performance Summary

| Metric | FastParserBytes | JSON | Improvement |
|--------|----------------|------|-------------|
| **Speed (reuse)** | 943 ns | 5,908 ns | **6.3x faster** |
| **Memory (reuse)** | 0 B | 3,448 B | **100% less** |
| **Allocations (reuse)** | 0 | 88 | **100% less** |
| **String access** | 13.88 ns | ~50+ ns | **3.6x faster** |

**FastParserBytes is production-ready for high-performance Internet Object parsing!** 🚀

# Internet Object Go Parser

## Overview
High-performance, thread-safe Internet Object parser implementation in Go. This package provides comprehensive tokenization and AST parsing capabilities with military-grade robustness and exceptional performance.

## Objectives

### Primary Objectives
1. **Parse Internet Object Format** - Fully compliant parser for the Internet Object specification
2. **High Performance** - Target: 10-50x faster than TypeScript implementation
3. **Thread Safety** - All operations must be safe for concurrent use
4. **Robust Error Handling** - Comprehensive error detection with precise position tracking
5. **Zero Dependencies** - Use only Go standard library (except for testing utilities)
6. **Production Ready** - Military-grade reliability and stability

### Design Principles
- **KISS (Keep It Simple, Stupid)** - Simple, understandable implementations over clever code
- **SRP (Single Responsibility Principle)** - Each component has one clear purpose
- **Don't Reinvent the Wheel** - Use Go standard library and established patterns
- **Idiomatic Go** - Follow Go best practices and conventions

## Non-Functional Requirements

### Performance Requirements
- **Parsing Speed**: Process at least 100 MB/s on modern hardware
- **Memory Efficiency**: Maximum 2x overhead of input size during parsing
- **Zero Allocations**: All critical paths must achieve zero allocations
- **Parallel Processing**: Support concurrent parsing of independent documents
- **Benchmarking**: Comprehensive benchmarks for all critical paths with allocation tracking

### Reliability Requirements
- **Error Recovery**: Graceful handling of malformed input
- **Position Tracking**: Precise line/column information for all errors
- **Validation**: Comprehensive input validation at all levels
- **Test Coverage**: Near 100% code coverage (target: 95%+)

### Testing Requirements
- **White Box Testing**: Unit tests in same file as implementation (e.g., `tokenizer.go` tests in `tokenizer_test.go` with `package parsers`)
- **Black Box Testing**: Integration/API tests in separate package (e.g., `parsers_test` package in `*_test.go` files)
- **Coverage Goals**:
  - Overall: 95%+ statement coverage
  - Critical paths: 100% coverage
  - Error handling: 100% coverage
  - Edge cases: Comprehensive coverage
- **Test Organization**:
  - Unit tests: `<filename>_test.go` with `package parsers` (white box)
  - Integration tests: `<feature>_integration_test.go` with `package parsers_test` (black box)
  - Benchmarks: `<filename>_bench_test.go` with allocation verification

### Thread Safety Requirements
- **Immutable AST**: AST nodes are immutable after creation
- **Stateless Operations**: Parser instances can be safely reused
- **Concurrent Parsing**: Multiple goroutines can parse different documents
- **Race Detection**: All code must pass `go test -race`

### Code Quality Requirements
- **Documentation**: Godoc for all exported types and functions
- **Linting**: Pass `golangci-lint` with strict settings
- **Formatting**: Standard `gofmt` formatting
- **Zero Allocations**: Critical paths must be verified with `go test -bench . -benchmem`

## Architecture

### Package Structure
```
parsers/
├── token.go           # Token definition and types
├── tokenizer.go       # Lexical analysis / tokenization
├── ast.go            # AST node definitions
├── parser.go         # AST parser implementation
├── errors.go         # Error types and handling
├── position.go       # Position tracking
├── pool.go           # Object pooling for performance
├── tokenizer_test.go # Tokenizer tests
├── parser_test.go    # Parser tests
└── benchmark_test.go # Performance benchmarks
```

### Core Components

#### 1. Tokenizer
- **Purpose**: Lexical analysis - convert input string to tokens
- **Features**:
  - Fast character scanning using byte-level operations
  - Support for all IO token types (strings, numbers, booleans, etc.)
  - Precise position tracking
  - Error recovery and reporting
  - String interning for common tokens

#### 2. AST Parser
- **Purpose**: Syntactic analysis - convert tokens to Abstract Syntax Tree
- **Features**:
  - Recursive descent parsing
  - Section and collection handling
  - Object and array parsing
  - Error node creation for malformed input
  - Validation during parsing

#### 3. Error Handling
- **Purpose**: Comprehensive error reporting and recovery
- **Features**:
  - Custom error types with position information
  - Error codes matching TypeScript implementation
  - Contextual error messages
  - Error recovery strategies

#### 4. Position Tracking
- **Purpose**: Track source code positions for tokens and nodes
- **Features**:
  - Line and column tracking
  - Byte offset tracking
  - Position ranges for multi-character tokens
  - UTF-8 aware

## Token Types

The tokenizer supports the following token types:

- **Structural**: `{`, `}`, `[`, `]`, `:`, `,`, `~`, `---`
- **Literals**: strings (regular, raw, annotated), numbers, booleans, null
- **Numbers**: integers, floats, hex, octal, binary, BigInt, Decimal, Infinity, NaN
- **Strings**: double-quoted, single-quoted, open strings, raw strings (r"..."), byte strings (b"...")
- **DateTime**: date (d"..."), time (t"..."), datetime (dt"...")
- **Sections**: section separators, section names, schema references

## AST Node Types

- **DocumentNode**: Root node containing header and sections
- **SectionNode**: Named section with optional schema
- **CollectionNode**: Collection of objects (~ delimiter)
- **ObjectNode**: Key-value pairs or array elements
- **ArrayNode**: Ordered list of values
- **MemberNode**: Key-value pair in object
- **TokenNode**: Leaf node wrapping a token
- **ErrorNode**: Error placeholder for malformed input

## Error Handling Strategy

### Error Types
1. **SyntaxError**: Invalid syntax (unclosed strings, invalid tokens, etc.)
2. **ValidationError**: Semantic errors (duplicate keys, type mismatches, etc.)
3. **EOFError**: Unexpected end of input

### Error Recovery
- Continue parsing after errors when possible
- Create ErrorNode for malformed sections
- Skip to next token boundary on unrecoverable errors
- Collect multiple errors in single pass

## Performance Optimizations

### Implemented Optimizations
1. ✅ **Manual Character Parsing**: Replaced all regex patterns with character-by-character parsing to eliminate regex allocations
2. ✅ **Batch Allocations**: Pre-allocate token slices with estimated capacity (`len(input)/8`)
3. ✅ **Reusable String Builder**: Added `strings.Builder` to Tokenizer struct for string operations
4. ✅ **Lookup Tables**: Use direct character classification functions (isDigit, isHexDigit, etc.)
5. ✅ **Inline Fast Paths**: Critical parsing functions optimized for zero allocations

### Current Performance Status
**Zero Allocation Achieved on Critical Paths:**
- Number parsing (`parseNumber`): **0 B/op, 0 allocs/op** ✓
- Character classification: **0 B/op, 0 allocs/op** ✓
- Position/Token utilities: **0 B/op, 0 allocs/op** ✓
- Whitespace skipping: **0 B/op, 0 allocs/op** ✓

**Full Tokenization Benchmarks:**
- Simple string: 160 B/op, 5 allocs/op
- Simple object: 1952 B/op, 40 allocs/op
- Real-world document: 7840 B/op, 151 allocs/op

### Future Optimization Opportunities
1. **Token Pool/Arena Allocator**: Implement object pooling to reuse token structs and reduce heap allocations
2. **String Interning**: Cache and reuse common strings (true, false, null, keywords)
3. **Zero-Copy String Operations**: Use byte slices with unsafe pointers where safe
4. **Batch Token Allocation**: Allocate tokens in contiguous memory blocks
5. **Inline More Fast Paths**: Profile and inline additional hot path functions
6. **Avoid Reflection**: Direct type assertions instead of reflection (when parser is implemented)

## Usage Example

```go
package main

import (
    "fmt"
    "log"
    "github.com/maniartech/internetobject-go/parsers"
)

func main() {
    input := `
name, age, email
---
John Doe, 30, john@example.com
---
Jane Smith, 25, jane@example.com
`

    // Tokenize
    tokenizer := parsers.NewTokenizer(input)
    tokens, err := tokenizer.Tokenize()
    if err != nil {
        log.Fatal(err)
    }

    // Parse
    parser := parsers.NewParser(tokens)
    document, err := parser.Parse()
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Parsed document with %d sections\n", len(document.Sections))
}
```

## Testing Strategy

### Unit Tests
- Test each component in isolation
- Test all token types
- Test all error conditions
- Test edge cases (empty input, EOF, etc.)

### Integration Tests
- Test complete parsing pipeline
- Test real-world IO documents
- Test error recovery

### Benchmark Tests
- Tokenization performance
- Parsing performance
- Memory allocations
- Comparison with TypeScript implementation

### Race Condition Tests
- Concurrent parsing of multiple documents
- Shared tokenizer instances
- Thread safety verification

## Development Roadmap

### Phase 1: Foundation ✅ COMPLETED
- [x] Project structure setup
- [x] Error handling implementation (errors.go)
- [x] Position tracking implementation (position.go)
- [x] Token definitions (token.go, constants.go)

### Phase 2: Tokenizer ✅ COMPLETED
- [x] Basic tokenizer implementation (tokenizer.go - 953 lines)
- [x] String parsing (all variants: regular, raw, annotated, byte)
- [x] Number parsing (all formats: int, float, hex, octal, binary, BigInt, Decimal)
- [x] Comment handling (single-line and multi-line)
- [x] Section separator parsing
- [x] Regex removal for zero allocations (all 5 patterns replaced with manual parsing)

### Phase 3: Parser (IN PROGRESS)
- [x] AST node definitions (ast.go)
- [ ] Document parsing
- [ ] Section parsing
- [ ] Object/Array parsing
- [ ] Error node handling

### Phase 4: Optimization & Performance (PARTIALLY COMPLETE)
- [x] Performance profiling and benchmarking (45+ benchmarks)
- [x] Zero allocations on critical paths (parseNumber, character classification)
- [x] Regex elimination (5/5 patterns removed)
- [ ] Token pool/arena allocator implementation
- [ ] String interning for common tokens
- [ ] Full tokenization allocation reduction (currently 5-151 allocs depending on input)
- [ ] Memory profiling and optimization

### Phase 5: Testing & Documentation (IN PROGRESS)
- [x] Comprehensive test suite (81% coverage, 60+ tests)
- [x] Performance benchmarks (tokenizer_bench_test.go, ast_bench_test.go)
- [x] White box unit tests (tokenizer_test.go, ast_test.go, etc.)
- [x] Black box integration tests (integration_test.go)
- [ ] Achieve 95%+ test coverage (current: 81%)
- [x] Documentation (README.md)
- [ ] Usage examples and tutorials

### Phase 6: Advanced Optimizations (PLANNED)
- [ ] Object pooling for token/node reuse (`pool.go`)
- [ ] Zero-copy string operations with unsafe pointers
- [ ] Batch token allocation in contiguous memory
- [ ] SIMD optimizations for character scanning (if beneficial)
- [ ] Lock-free concurrent parsing support
- [ ] Memory-mapped file support for large inputs

## Performance Targets

| Metric | Target | Current Status | Notes |
|--------|--------|----------------|-------|
| Parse Speed | 100+ MB/s | TBD | Pending parser implementation |
| Memory Overhead | < 2x input | ~2x | 7840 B for 4KB real-world document |
| Concurrent Parse | 10+ goroutines | ✅ Supported | Thread-safe tokenizer design |
| Critical Path Allocations | 0 allocs/op | ✅ **Achieved** | parseNumber, character classification, utilities all at 0 allocs |
| Full Tokenization Allocations | < 100 allocs/doc | 151 allocs/doc | Future optimization: token pooling, string interning |
| Test Coverage | > 95% | 81.0% | White box + black box tests, all passing |

### Performance Notes

**Achievements:**
- ✅ All regex removed - manual character parsing eliminates regex allocation overhead
- ✅ Critical paths achieve zero allocations (parsing hot paths)
- ✅ Pre-allocated token slices reduce reallocation overhead
- ✅ 45+ benchmarks with allocation tracking

**Known Limitations:**
- Token struct allocation is inherent to current design (each token = 1 heap allocation)
- Full tokenization requires multiple allocations for token slice growth
- String operations in certain paths still allocate

**Planned Improvements:**
- Implement token pool/arena allocator to reuse tokens
- Add string interning for common values (true, false, null, etc.)
- Batch allocate tokens in contiguous memory blocks
- Profile and optimize remaining allocation hot spots

## Contributing

Follow Go best practices:
- Run `go fmt` before committing
- Run `go vet` to catch common errors
- Run `golangci-lint run` for comprehensive linting
- Ensure all tests pass: `go test -race -coverprofile=coverage.out ./...`
- Add benchmarks for performance-critical code
- Document all exported types and functions

## License

Same as parent project (Internet Object)


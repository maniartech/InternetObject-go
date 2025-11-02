# Zero-Copy Token Extraction Methods

## Overview

ZeroParser provides multiple methods for extracting token data, optimized for different use cases. The choice depends on your performance requirements and memory constraints.

## Performance Comparison

**Benchmark Results** (extracting 7 tokens repeatedly):

| Method | Time (ns/op) | Memory (B/op) | Allocations | Use Case |
|--------|--------------|---------------|-------------|----------|
| `GetTokenBytes()` | **10.53** | 0 | 0 | **Fastest - read-only access** |
| `CopyTokenBytes()` | 28.62 | 0 | 0 | Safe copy to reusable buffer |
| `GetTokenBytesTo()` | 28.59 | 0 | 0 | Optimized copy variant |
| `GetTokenString()` | 37.83 | 0 | 0 | Convenience (returns string) |

**Winner: `GetTokenBytes()` is 3.6x faster than `GetTokenString()`!** 🏆

## Method Details

### 1. GetTokenBytes() - Zero-Copy Reference ⚡

**Fastest method - returns direct reference to parser's internal buffer.**

```go
func (p *ZeroParser) GetTokenBytes(tokenIdx uint32) []byte
```

**Pros:**
- ✅ Zero allocations
- ✅ Zero copies
- ✅ **3.6x faster than string conversion**
- ✅ Perfect for read-only operations

**Cons:**
- ⚠️ Returned slice references internal buffer (becomes invalid if parser is GC'd)
- ⚠️ Must not modify the returned slice

**Example:**
```go
parser := NewZeroParser(`{name: "Alice", age: 25}`)
rootIdx, _ := parser.Parse()

// Get zero-copy reference
tokenBytes := parser.GetTokenBytes(someTokenIdx)
fmt.Printf("Value: %s\n", tokenBytes) // No allocation!

// Compare directly
if bytes.Equal(tokenBytes, []byte("Alice")) {
    // Fast comparison
}
```

**Best for:**
- Reading token values
- Comparisons with known byte sequences
- Passing to functions that accept `[]byte`
- High-performance loops

---

### 2. CopyTokenBytes() - Safe Copy 🛡️

**Copies token data to your pre-allocated buffer.**

```go
func (p *ZeroParser) CopyTokenBytes(tokenIdx uint32, dst []byte) int
```

**Pros:**
- ✅ Safe - can outlive the parser
- ✅ Reusable buffer (zero allocations if buffer is pre-allocated)
- ✅ Returns required size if buffer is too small

**Cons:**
- ⚠️ Slightly slower due to copy operation (but still zero allocations)

**Example:**
```go
parser := NewZeroParser(`{name: "Bob", age: 30}`)
parser.Parse()

// Pre-allocate reusable buffer
buf := make([]byte, 256)

for idx := uint32(0); idx < uint32(parser.tokenCount); idx++ {
    n := parser.CopyTokenBytes(idx, buf)
    if n > 0 {
        value := buf[:n]
        // Process value (safe to use after parser is gone)
    }
}
```

**Best for:**
- Values that need to outlive the parser
- Reusing a single buffer in loops
- Building strings/values from multiple tokens

---

### 3. GetTokenBytesTo() - Optimized Copy ⚡

**Writes token data directly to your buffer (fastest copy variant).**

```go
func (p *ZeroParser) GetTokenBytesTo(tokenIdx uint32, dst []byte) int
```

**Pros:**
- ✅ Slightly faster than `CopyTokenBytes()`
- ✅ Zero allocations
- ✅ Panics if buffer is too small (caller must ensure correct size)

**Example:**
```go
parser := NewZeroParser(`{count: 42}`)
parser.Parse()

// Ensure buffer is large enough
tok := parser.tokens[tokenIdx]
bufSize := int(tok.End - tok.Start)
buf := make([]byte, bufSize)

n := parser.GetTokenBytesTo(tokenIdx, buf)
value := buf[:n]
```

**Best for:**
- When you know the exact size needed
- Hot paths where panic on size mismatch is acceptable
- Maximizing performance with pre-sized buffers

---

### 4. GetTokenString() - Convenience 📦

**Returns a Go string (allocates memory for the string).**

```go
func (p *ZeroParser) GetTokenString(tokenIdx uint32) string
```

**Pros:**
- ✅ Convenient - returns native string type
- ✅ Safe - string is independent of parser
- ✅ Easy to use

**Cons:**
- ⚠️ Allocates memory for string (though Go optimizes small strings)
- ⚠️ 3.6x slower than `GetTokenBytes()`

**Example:**
```go
parser := NewZeroParser(`{name: "Charlie"}`)
rootIdx, _ := parser.Parse()

// Simple and convenient
tokenStr := parser.GetTokenString(someTokenIdx)
fmt.Printf("Name: %s\n", tokenStr) // Allocates string
```

**Best for:**
- Simple use cases
- When convenience matters more than performance
- Displaying values to users
- Interfacing with APIs that require `string`

---

## Performance Recommendations

### 🏃 Maximum Performance (Hot Paths)

```go
// Use GetTokenBytes() for read-only access
tokenBytes := parser.GetTokenBytes(idx)
if bytes.Equal(tokenBytes, expectedValue) {
    // Process...
}
```

### 🔄 Reusable Buffer Pattern

```go
// Pre-allocate once, reuse many times
buf := make([]byte, 1024)

for idx := range parser.tokens {
    n := parser.CopyTokenBytes(uint32(idx), buf)
    processValue(buf[:n])
}
```

### 🎯 Direct Buffer Write (Fastest Copy)

```go
// When you know the size
tok := parser.tokens[idx]
buf := make([]byte, tok.End-tok.Start)
n := parser.GetTokenBytesTo(uint32(idx), buf)
```

### 📦 Simple/Convenient

```go
// When readability matters
name := parser.GetTokenString(nameTokenIdx)
fmt.Printf("Hello, %s!\n", name)
```

---

## Real-World Example: JSON-like Processing

```go
type User struct {
    Name  string
    Email string
    Age   int
}

func ParseUser(parser *ZeroParser, objNodeIdx uint32) User {
    user := User{}
    objNode := parser.nodes[objNodeIdx]

    // Pre-allocate buffer for reuse
    buf := make([]byte, 256)

    // Iterate members
    for i := uint32(0); i < uint32(objNode.ChildCount); i++ {
        memberIdx := parser.childIndices[objNode.ChildStart+i]
        memberNode := parser.nodes[memberIdx]

        // Get key using zero-copy reference (read-only)
        keyBytes := parser.GetTokenBytes(memberNode.TokenIdx)

        // Get value node
        valueNodeIdx := parser.childIndices[memberNode.ChildStart]
        valueNode := parser.nodes[valueNodeIdx]
        valueTokenIdx := valueNode.TokenIdx

        // Switch based on key (zero-copy comparison!)
        switch {
        case bytes.Equal(keyBytes, []byte("name")):
            // Copy to buffer for string conversion
            n := parser.CopyTokenBytes(valueTokenIdx, buf)
            user.Name = string(buf[:n])

        case bytes.Equal(keyBytes, []byte("email")):
            n := parser.CopyTokenBytes(valueTokenIdx, buf)
            user.Email = string(buf[:n])

        case bytes.Equal(keyBytes, []byte("age")):
            // For numbers, zero-copy reference is fine
            ageBytes := parser.GetTokenBytes(valueTokenIdx)
            user.Age, _ = strconv.Atoi(string(ageBytes))
        }
    }

    return user
}
```

**This approach:**
- Uses `GetTokenBytes()` for keys (read-only comparison)
- Uses `CopyTokenBytes()` with reusable buffer for values
- **Zero allocations in the loop!**
- **Maximum performance with safety**

---

## Summary

| Your Need | Recommended Method | Reason |
|-----------|-------------------|--------|
| **Fastest possible** | `GetTokenBytes()` | 10.53 ns/op, zero-copy |
| **Read-only access** | `GetTokenBytes()` | Direct reference, no allocations |
| **Need to keep value** | `CopyTokenBytes()` | Safe copy, reusable buffer |
| **Hot path with known size** | `GetTokenBytesTo()` | Optimized copy |
| **Simple/Convenient** | `GetTokenString()` | Returns native string |
| **Comparison operations** | `GetTokenBytes()` + `bytes.Equal()` | Fastest comparison |
| **Building results** | `CopyTokenBytes()` + reusable buffer | Zero allocations in loop |

**Golden Rule:** Use `GetTokenBytes()` whenever possible, fall back to `CopyTokenBytes()` when you need to keep the data, and use `GetTokenString()` only for convenience or API compatibility.

package parsers

// FastTokenizer is an optimized tokenizer that minimizes allocations
// by using token value arrays instead of pointers
type FastTokenizer struct {
input       string
pos         int
row         int
col         int
inputLength int
tokens      []Token  // Value array instead of pointer array
}

// NewFastTokenizer creates an optimized tokenizer
func NewFastTokenizer(input string) *FastTokenizer {
capacity := len(input) / 6 // Better estimate: avg 6 chars per token
if capacity < 16 {
capacity = 16
}
return &FastTokenizer{
input:       input,
inputLength: len(input),
row:         1,
col:         1,
tokens:      make([]Token, 0, capacity),
}
}

// TokenizeValues returns token values instead of pointers
func (t *FastTokenizer) TokenizeValues() ([]Token, error) {
// Use the existing Tokenizer but convert to values
oldTok := NewTokenizer(t.input)
ptrs, err := oldTok.Tokenize()
if err != nil {
return nil, err
}

// Convert to values
values := make([]Token, len(ptrs))
for i, ptr := range ptrs {
values[i] = *ptr
}
return values, nil
}

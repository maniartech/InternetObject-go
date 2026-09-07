package tokenizer

// Stream is the result of tokenizing one input: the normalized source (CRLF
// and CR become LF) and its tokens in order.
type Stream struct {
	Src    string
	Tokens []Token
}

// Text returns the token's raw source text.
func (s *Stream) Text(t Token) string { return s.Src[t.Start:t.End] }

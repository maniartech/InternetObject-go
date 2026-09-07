// Package tokenizer turns Internet Object source text into a flat token
// stream.
//
// Tokens are compact value structs holding only classification and positions;
// decoded payloads (unescaped strings, parsed numbers, byte slices) are
// produced on demand by the Stream decode methods, so steady-state
// tokenization allocates nothing per token. Errors are never returned: a
// malformed construct becomes an ERROR token carrying a stable error code,
// because Internet Object accumulates errors rather than failing fast.
package tokenizer

// Token is one lexical token: classification plus positions into the stream's
// normalized source. It carries no decoded payload — decode through the
// Stream. Twenty bytes, held by value in the stream's slice.
type Token struct {
	Kind Kind
	Sub  Sub
	Err  Code // set only when Kind == KindError

	Start, End int32 // byte offsets into Stream.Src (half-open)
	Line, Col  int32 // 1-based position of Start
}

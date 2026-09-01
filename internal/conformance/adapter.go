package conformance

import (
	"github.com/maniartech/InternetObject-go/internal/numfmt"
	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// The bootstrap adapter: renders this tokenizer's stream in the corpus's
// all-text token vocabulary. The `type` column decides how `value` reads —
// BINARY compares the base64 text, ERROR compares nothing (the code is its
// own column), and object-valued tokens (DECIMAL, DATETIME, NULL) compare as
// the empty string.
func init() {
	tokenizeHook = func(input string) []TokenFields {
		s := tokenizer.Tokenize(input)
		out := make([]TokenFields, 0, len(s.Tokens))
		for _, t := range s.Tokens {
			f := TokenFields{
				Type:      t.Kind.String(),
				SubType:   t.Sub.String(),
				Token:     s.Text(t),
				ErrorCode: t.Err.String(),
			}
			switch t.Kind {
			case tokenizer.KindString:
				f.Value = s.StringValue(t)
			case tokenizer.KindNumber:
				f.Value = numfmt.Format(s.Number(t))
			case tokenizer.KindBigInt:
				f.Value = s.BigInt(t).String()
			case tokenizer.KindBoolean:
				if s.Bool(t) {
					f.Value = "true"
				} else {
					f.Value = "false"
				}
			case tokenizer.KindBinary:
				f.Value = s.Base64Text(t)
			case tokenizer.KindCurlyOpen, tokenizer.KindCurlyClose,
				tokenizer.KindBracketOpen, tokenizer.KindBracketClose,
				tokenizer.KindColon, tokenizer.KindComma,
				tokenizer.KindCollectionStart, tokenizer.KindSectionSep:
				f.Value = s.Text(t)
			}
			out = append(out, f)
		}
		return out
	}
}

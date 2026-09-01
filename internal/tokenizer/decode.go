package tokenizer

import (
	"encoding/base64"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// On-demand token decoding. Tokens carry only positions; these methods produce
// the decoded value from the source text, allocating only when the text and
// the value genuinely differ (escapes, doubled quotes, exponents).
//
// Every method assumes the token was produced by this stream's tokenizer and
// is of the matching kind — they are the single decode site for each literal
// form, shared by the conformance harness and (later) the parser.

// Bool decodes a BOOLEAN token.
func (s *Stream) Bool(t Token) bool {
	c := s.Src[t.Start]
	return c == 't' || c == 'T'
}

// Number decodes a NUMBER token to a float64.
func (s *Stream) Number(t Token) float64 {
	txt := s.Text(t)
	switch txt {
	case "NaN":
		return math.NaN()
	case "Inf", "+Inf":
		return math.Inf(1)
	case "-Inf":
		return math.Inf(-1)
	}
	if body, neg, base := splitBased(txt); base != 0 {
		f := basedFloat(body, base)
		if neg {
			return -f
		}
		return f
	}
	f, _ := strconv.ParseFloat(txt, 64)
	return f
}

// basedFloat converts base-prefixed digits to a float, exactly for magnitudes
// a uint64 holds and to nearest beyond that.
func basedFloat(digits string, base int) float64 {
	if u, err := strconv.ParseUint(digits, base, 64); err == nil {
		return float64(u)
	}
	bi, _ := new(big.Int).SetString(digits, base)
	f, _ := bi.Float64()
	return f
}

// splitBased splits an optional sign and a base prefix from a numeric word.
// base is 0 when the word carries no base prefix.
func splitBased(txt string) (digits string, neg bool, base int) {
	body := txt
	switch body[0] {
	case '-':
		neg = true
		body = body[1:]
	case '+':
		body = body[1:]
	}
	if len(body) >= 2 && body[0] == '0' {
		if b := baseFor(body[1]); b != 0 {
			return body[2:], neg, b
		}
	}
	return body, neg, 0
}

// BigInt decodes a BIGINT token.
func (s *Stream) BigInt(t Token) *big.Int {
	txt := s.Text(t)
	txt = txt[:len(txt)-1] // strip the n suffix
	digits, neg, base := splitBased(txt)
	var bi *big.Int
	if base != 0 {
		bi, _ = new(big.Int).SetString(digits, base)
	} else if mant, exp, found := cutExponent(digits); found {
		bi, _ = new(big.Int).SetString(mant, 10)
		e, _ := strconv.Atoi(strings.TrimPrefix(exp, "+"))
		bi.Mul(bi, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(e)), nil))
	} else {
		bi, _ = new(big.Int).SetString(digits, 10)
	}
	if neg {
		bi.Neg(bi)
	}
	return bi
}

func cutExponent(s string) (mant, exp string, found bool) {
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, "", false
}

// DecimalParts decodes a DECIMAL token into its coefficient digits and scale:
// 1.50m → coefficient 150, scale 2. Scale is part of the value.
func (s *Stream) DecimalParts(t Token) (coef *big.Int, scale int) {
	txt := s.Text(t)
	txt = txt[:len(txt)-1] // strip the m suffix
	if dot := strings.IndexByte(txt, '.'); dot >= 0 {
		scale = len(txt) - dot - 1
		txt = txt[:dot] + txt[dot+1:]
	}
	coef, _ = new(big.Int).SetString(txt, 10)
	return coef, scale
}

// StringValue decodes any STRING token to its string value.
func (s *Stream) StringValue(t Token) string {
	switch t.Sub {
	case SubRegularString:
		return decodeRegular(s.innerText(t, 0))
	case SubRawString:
		inner := s.innerText(t, 1)
		q := s.Src[t.Start+1]
		return collapseDoubled(inner, q)
	default: // open string, section name: the text is the value
		return s.Text(t)
	}
}

// innerText returns the token text between the quotes, where prefixLen counts
// the bytes before the opening quote (0 for a bare quote, 1 for r/b/d/t, 2
// for dt).
func (s *Stream) innerText(t Token, prefixLen int32) string {
	return s.Src[t.Start+prefixLen+1 : t.End-1]
}

// Base64Text returns a BINARY token's payload as the base64 text itself.
func (s *Stream) Base64Text(t Token) string { return s.innerText(t, 1) }

// Bytes decodes a BINARY token's payload. Missing padding is tolerated.
func (s *Stream) Bytes(t Token) []byte {
	text := strings.TrimRight(s.Base64Text(t), "=")
	b, _ := base64.RawStdEncoding.DecodeString(text)
	return b
}

// Temporal decodes a DATETIME token. The token's Sub carries the kind, which
// stays distinct from the instant.
func (s *Stream) Temporal(t Token) time.Time {
	prefixLen := int32(1)
	if t.Sub == SubDateTime {
		prefixLen = 2
	}
	tm, _ := parseTemporal(s.innerText(t, prefixLen), t.Sub)
	return tm.timeValue(t.Sub)
}

// decodeRegular processes the escape sequences of a regular string's inner
// text. Escapes are lenient except the marker escapes \u and \x, which the
// scanner already validated.
func decodeRegular(inner string) string {
	bs := strings.IndexByte(inner, '\\')
	if bs < 0 {
		return inner
	}
	out := make([]byte, 0, len(inner))
	out = append(out, inner[:bs]...)
	for i := bs; i < len(inner); {
		c := inner[i]
		if c != '\\' {
			out = append(out, c)
			i++
			continue
		}
		if i+1 >= len(inner) {
			out = append(out, c) // trailing backslash (scanner errors this case)
			break
		}
		e := inner[i+1]
		if e >= utf8.RuneSelf {
			i++ // drop the backslash; the rune's own bytes follow literally
			continue
		}
		i += 2
		switch e {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'u':
			cp, _ := strconv.ParseUint(inner[i:i+4], 16, 32)
			i += 4
			r := rune(cp)
			// combine a UTF-16 surrogate pair written as two \u escapes
			if utf16.IsSurrogate(r) && i+6 <= len(inner) && inner[i] == '\\' && inner[i+1] == 'u' {
				if lo, err := strconv.ParseUint(inner[i+2:i+6], 16, 32); err == nil {
					if c := utf16.DecodeRune(r, rune(lo)); c != utf8.RuneError {
						r = c
						i += 6
					}
				}
			}
			out = utf8.AppendRune(out, r) // a lone surrogate becomes U+FFFD
		case 'x':
			cp, _ := strconv.ParseUint(inner[i:i+2], 16, 32)
			i += 2
			out = utf8.AppendRune(out, rune(cp))
		default:
			// unrecognized escape: the backslash is dropped, the character kept
			out = append(out, e)
		}
	}
	return string(out)
}

// collapseDoubled rewrites a raw string's doubled enclosing quotes to single
// ones. When none are present, the inner text is returned as-is (no copy).
func collapseDoubled(inner string, q byte) string {
	i := strings.IndexByte(inner, q)
	if i < 0 {
		return inner
	}
	out := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); i++ {
		out = append(out, inner[i])
		if inner[i] == q {
			i++ // skip the second quote of the pair
		}
	}
	return string(out)
}

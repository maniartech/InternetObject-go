package document

import (
	"strings"
	"unicode/utf8"

	"github.com/maniartech/InternetObject-go/internal/tokenizer"
)

// Writing KEYS: member keys and header names, which quote by different rules
// than values do.

func headerName(name string) string {
	var dst []byte
	for i := 0; i < len(name); {
		c := name[i]
		if c < utf8.RuneSelf {
			switch {
			case c == '\\' || tokenizer.IsTerminatorByte(c) || c == ' ':
				dst = append(dst, '\\', c)
			case c < 0x20:
				dst = appendControlEscape(dst, c)
			case c == '-' && i+2 < len(name) && name[i+1] == '-' && name[i+2] == '-':
				dst = append(dst, '\\', c)
			default:
				dst = append(dst, c)
			}
			i++
			continue
		}
		r, w := utf8.DecodeRuneInString(name[i:])
		if tokenizer.IsSpaceRune(r) {
			dst = append(dst, '\\') // the rune itself then flows as content
		}
		dst = append(dst, name[i:i+w]...)
		i += w
	}
	return string(dst)
}

// formatObjectKey quotes a key whose bare spelling would not read back as
// that key: numerics, keywords, and anything outside the identifier-like set.
func formatObjectKey(key string) string {
	return string(appendObjectKey(nil, key))
}

// keyIsBare reports whether a key can be written unquoted — THE key-quoting
// decision, made once and shared by both spellings.
func keyIsBare(key string) bool {
	return isBareSafeKey(key) &&
		!strings.HasSuffix(key, " ") && !strings.Contains(key, "---") &&
		!isNumericKey(key) && !keywordKeys[key]
}

// appendObjectKey is formatObjectKey in append form.
func appendObjectKey(dst []byte, key string) []byte {
	if keyIsBare(key) {
		return append(dst, key...)
	}
	return appendRegularString(dst, key)
}

// isSpaceByte reports ASCII whitespace — the word separators a bare run can
// carry (multi-byte Unicode spaces never appear inside one unquoted word).
func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}

// ── exported spelling helpers ──────────────────────────────────────────────
//
// These are THE sites that decide how a value is spelled. A caller that walks
// Go values directly (the marshaler's fast path) appends through these, so
// there is exactly one implementation of every quoting, number and temporal
// rule no matter which traversal produced the value (ADR 0006 roadmap item 5).

// AppendString appends a string in its leanest safe spelling.

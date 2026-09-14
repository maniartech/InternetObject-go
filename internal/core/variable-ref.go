package core

import "strings"

// IsVariableRef reports whether a string VALUE is a reference to a header
// variable: `@` followed by at least one character. A lone `@` is just text.
//
// The rule applies to a string in any spelling — open or quoted — by design
// (io-test-cases FINDINGS #3), and every place that asks the question must
// give the same answer, so this is the one statement of it. It used to be
// written out at five sites. When the lazy decoder did not ask at all, it bound
// `@0` as the literal text "@0" where the general path reports
// undefined-variable — a validation bypass found by the differential fuzzer on
// 2026-09-14.
func IsVariableRef(s string) bool {
	return len(s) > 1 && strings.HasPrefix(s, "@")
}

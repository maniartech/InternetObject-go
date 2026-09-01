package conformance

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

// The bootstrap comparator — io-test-cases/bootstrap/tokenizer.csv run against
// this tokenizer. The CSV exists to break the circularity of a corpus written
// in the format under test: it is readable with nothing but the standard
// library, so a port has a working conformance harness before it has a parser.
//
// Format (see io-test-cases/bootstrap/README.md): one row per expected token,
// grouped into cases by (suite, case). A case expecting no tokens is a single
// row with an empty index. Cells use a small escape convention (\\ \n \r \t
// \uXXXX) so control characters survive the CSV.

// TokenFields is a token as the corpus spells it: every field is text, so any
// implementation can compare. The type column says how to read value — for
// BINARY it is the base64 text, for ERROR it is empty (the code is in
// ErrorCode).
type TokenFields struct {
	Type      string
	SubType   string
	Value     string
	Token     string
	ErrorCode string
}

// TokenCase is one bootstrap case: an input and its ordered expected tokens.
type TokenCase struct {
	Suite    string
	Name     string
	Input    string
	Expected []TokenFields
}

// tokenizeHook produces this implementation's token stream for one input,
// rendered as corpus fields. It is nil until the tokenizer exists — phase 0
// reports an honest 0/N rather than a vacuous pass.
var tokenizeHook func(input string) []TokenFields

// unescapeCell reverses the generator's escape convention. Nothing else is
// escaped: `\\` is a backslash, `\n` `\r` `\t` are the controls, `\uXXXX` is
// any other control character, and any other `\x` pair passes x through.
func unescapeCell(cell string) string {
	i := 0
	for ; i < len(cell); i++ {
		if cell[i] == '\\' {
			break
		}
	}
	if i == len(cell) {
		return cell
	}
	out := make([]byte, 0, len(cell))
	out = append(out, cell[:i]...)
	for ; i < len(cell); i++ {
		if cell[i] != '\\' || i+1 == len(cell) {
			out = append(out, cell[i])
			continue
		}
		i++
		switch cell[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			if i+4 < len(cell) {
				if cp, err := strconv.ParseUint(cell[i+1:i+5], 16, 32); err == nil {
					out = append(out, string(rune(cp))...)
					i += 4
					continue
				}
			}
			out = append(out, 'u')
		default:
			out = append(out, cell[i])
		}
	}
	return string(out)
}

// LoadBootstrapCases parses the CSV into one case per (suite, case), each
// holding its ordered expected tokens.
func LoadBootstrapCases(file string) ([]TokenCase, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s: empty file", file)
	}

	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	get := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return row[i]
		}
		return ""
	}

	var cases []TokenCase
	index := map[string]int{} // (suite, case) -> position in cases
	for _, row := range rows[1:] {
		key := get(row, "suite") + "/" + get(row, "case")
		i, ok := index[key]
		if !ok {
			i = len(cases)
			index[key] = i
			cases = append(cases, TokenCase{
				Suite: get(row, "suite"),
				Name:  get(row, "case"),
				Input: unescapeCell(get(row, "input")),
			})
		}
		if get(row, "index") == "" {
			continue // the "no tokens expected" marker row
		}
		cases[i].Expected = append(cases[i].Expected, TokenFields{
			Type:      get(row, "type"),
			SubType:   get(row, "sub_type"),
			Value:     unescapeCell(get(row, "value")),
			Token:     unescapeCell(get(row, "token")),
			ErrorCode: get(row, "error_code"),
		})
	}
	return cases, nil
}

// RunTokenCase runs one case through the tokenizer. It returns the problems
// found; empty means the case passed.
func RunTokenCase(c TokenCase) []string {
	if tokenizeHook == nil {
		return []string{"tokenizer: not implemented"}
	}
	actual := tokenizeHook(c.Input)

	var problems []string
	if len(actual) != len(c.Expected) {
		problems = append(problems, fmt.Sprintf("token count: expected %d, got %d", len(c.Expected), len(actual)))
	}
	n := min(len(actual), len(c.Expected))
	for i := 0; i < n; i++ {
		want, got := c.Expected[i], actual[i]
		check := func(field, w, g string) {
			if w != g {
				problems = append(problems, fmt.Sprintf("[%d].%s: expected %q, got %q", i, field, w, g))
			}
		}
		check("type", want.Type, got.Type)
		check("subType", want.SubType, got.SubType)
		check("value", want.Value, got.Value)
		check("token", want.Token, got.Token)
		check("errorCode", want.ErrorCode, got.ErrorCode)
	}
	return problems
}

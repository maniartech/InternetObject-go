package conformance

import (
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/numfmt"
	"github.com/maniartech/InternetObject-go/internal/value"
)

// The `.io` corpus loader and the `parse`-kind comparator. The corpus is
// written in Internet Object itself, so this loader reads suite files with
// the very parser under test — the self-hosting step: a corpus only readable
// by a working implementation is also a corpus that proves one works.

// SuiteRow is one corpus case, projected to named columns by the suite's own
// $schema header.
type SuiteRow struct {
	Name          string
	Input         string
	HasInput      bool
	SchemaDef     string
	HasSchemaDef  bool
	Schema        string
	HasSchema     bool
	Expected      any
	Output        any
	HasOutput     bool
	Definitions   string
	DefaultSchema string
	ErrorCodes    []string
	Recovered     any
	HasRecovered  bool
}

// LoadIOSuite parses one suite file and returns its rows. A suite file that
// itself fails to parse is an error — the gate must be seen to run.
func LoadIOSuite(file string) ([]SuiteRow, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	doc := document.Load(string(src))
	if len(doc.Errors) > 0 {
		codes := make([]string, len(doc.Errors))
		for i, e := range doc.Errors {
			codes[i] = e.Code
		}
		return nil, fmt.Errorf("%s: the suite file itself does not parse: %s", file, strings.Join(codes, ", "))
	}

	projected := doc.Project()
	var raw []any
	switch v := projected.(type) {
	case []any:
		raw = v
	case *value.Object:
		if i := v.Find("data"); i >= 0 {
			raw, _ = v.Members[i].Value.([]any)
		}
	}

	rows := make([]SuiteRow, 0, len(raw))
	for _, r := range raw {
		obj, ok := r.(*value.Object)
		if !ok {
			continue
		}
		row := SuiteRow{}
		if v, ok := field(obj, "name"); ok {
			row.Name, _ = v.(string)
		}
		if v, ok := field(obj, "input"); ok {
			row.Input, _ = v.(string)
			row.HasInput = true
		}
		if v, ok := field(obj, "schemaDef"); ok {
			row.SchemaDef, _ = v.(string)
			row.HasSchemaDef = true
		}
		if v, ok := field(obj, "schema"); ok {
			row.Schema, _ = v.(string)
			row.HasSchema = true
		}
		row.Expected, _ = field(obj, "expected")
		row.Output, row.HasOutput = field(obj, "output")
		if v, ok := field(obj, "definitions"); ok {
			row.Definitions, _ = v.(string)
		}
		if v, ok := field(obj, "defaultSchema"); ok {
			row.DefaultSchema, _ = v.(string)
		}
		if v, ok := field(obj, "error_codes"); ok {
			if arr, ok := v.([]any); ok {
				for _, c := range arr {
					if s, ok := c.(string); ok {
						row.ErrorCodes = append(row.ErrorCodes, s)
					}
				}
			}
		}
		row.Recovered, row.HasRecovered = field(obj, "recovered")
		rows = append(rows, row)
	}
	return rows, nil
}

func field(o *value.Object, key string) (any, bool) {
	if i := o.Find(key); i >= 0 {
		return o.Members[i].Value, true
	}
	return nil, false
}

// RunParseCase runs one parse-kind case: parse the input, compare the
// accumulated error codes exactly (order included), and — when no errors are
// expected — the projected value. The `recovered` column, when present,
// additionally asserts the value that survived DESPITE the errors.
func RunParseCase(row SuiteRow) []string {
	doc := document.Load(row.Input)
	var codes []string
	for _, e := range doc.Errors {
		codes = append(codes, e.Code)
	}

	var problems []string
	if !stringsEqual(codes, row.ErrorCodes) {
		problems = append(problems, fmt.Sprintf("codes  expected=%v  actual=%v", row.ErrorCodes, codes))
	}
	if len(row.ErrorCodes) == 0 {
		var actual any
		if len(codes) == 0 {
			actual = doc.Project()
		}
		if !value.Equal(actual, row.Expected) {
			problems = append(problems, fmt.Sprintf("value  expected=%s  actual=%s", Show(row.Expected), Show(actual)))
		}
	}
	if row.HasRecovered && !value.Equal(doc.Project(), row.Recovered) {
		problems = append(problems, fmt.Sprintf("recovered  expected=%s  actual=%s", Show(row.Recovered), Show(doc.Project())))
	}
	return problems
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Show renders a value in the corpus's neutral spelling, for failure output.
// Object keys are sorted, mirroring the comparison.
func Show(v any) string {
	var b strings.Builder
	show(&b, v)
	return b.String()
}

func show(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		fmt.Fprintf(b, "%v", x)
	case float64:
		b.WriteString(numfmt.Format(x))
	case string:
		fmt.Fprintf(b, "%q", x)
	case *big.Int:
		fmt.Fprintf(b, "%sn", x.String())
	case value.Decimal:
		fmt.Fprintf(b, "dec(%s,%d)", x.Coef.String(), x.Scale)
	case []byte:
		fmt.Fprintf(b, "bytes(%x)", x)
	case value.Temporal:
		b.WriteString(x.T.UTC().Format("2006-01-02T15:04:05.000Z"))
	case value.ErrorNode:
		fmt.Fprintf(b, "errorNode(%s)", x.Code)
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			show(b, e)
		}
		b.WriteByte(']')
	case *value.Object:
		keys := make([]string, len(x.Members))
		for i := range x.Members {
			keys[i] = x.Members[i].Key
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(b, "%q:", k)
			show(b, x.Members[x.Find(k)].Value)
		}
		b.WriteByte('}')
	default:
		fmt.Fprintf(b, "?%T", v)
	}
}

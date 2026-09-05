package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
	"text/template"
	"unicode"

	"github.com/maniartech/InternetObject-go/internal/document"
	"github.com/maniartech/InternetObject-go/internal/schema"
)

// field is one member, resolved to the names and Go type the templates need.
type field struct {
	Member string // the IO member name, verbatim — the wire contract
	Go     string // the exported Go identifier
	priv   string // the unexported field name
	GoType string
}

// Priv is the unexported field name, exported for the template.
func (f field) Priv() string { return f.priv }

type unit struct {
	Package    string
	Type       string // the generated type name, e.g. "User"
	PrivType   string // its unexported form, e.g. "user" — names the plain twin
	Recv       string // its receiver, e.g. "u"
	CtorLocal  string // the constructor's local, distinct from every parameter
	SchemaText string // the verbatim schema source
	Fields     []field
	Imports    []string
}

// Generate compiles the schema, refuses anything it cannot bind exactly, and
// returns the formatted Go source for the guarded type and for its tests.
//
// The generated code contains ZERO semantic logic (ADR 0004 D6): every value
// crossing the wire goes through the engine's own MarshalWith/UnmarshalWith
// against this schema, and every guard is io.ValidateWith. This file decides
// names and Go types; it never decides what the format means.
func Generate(pkg, typeName, schemaText string) (code, tests []byte, err error) {
	s, cerr := document.ParseSchema(schemaText)
	if cerr != nil {
		return nil, nil, fmt.Errorf("schema does not compile: %s at %d:%d", cerr.Code, cerr.Line, cerr.Col)
	}
	if s == nil {
		return nil, nil, fmt.Errorf("schema is empty")
	}
	if s.Open != nil {
		return nil, nil, fmt.Errorf("an open schema (`*`) has no fixed shape to generate")
	}

	u := unit{
		Package:    pkg,
		Type:       typeName,
		PrivType:   unexported(typeName),
		SchemaText: schemaText,
	}
	for _, name := range s.Names {
		md := s.Defs[name]
		gt, err := goType(md)
		if err != nil {
			return nil, nil, fmt.Errorf("member %q: %w", name, err)
		}
		id, err := exportedIdent(name)
		if err != nil {
			return nil, nil, fmt.Errorf("member %q: %w", name, err)
		}
		if id == typeName {
			return nil, nil, fmt.Errorf("member %q collides with the type name", name)
		}
		u.Fields = append(u.Fields, field{
			Member: name, Go: id, priv: unexported(id), GoType: gt,
		})
	}
	if len(u.Fields) == 0 {
		return nil, nil, fmt.Errorf("schema declares no members")
	}

	// Every generated local must be distinct from every FIELD name, because a
	// member name is arbitrary user input and the constructor takes one
	// parameter per member. Two bugs the corpus gate caught: a member named
	// "t" collided with the receiver of a type starting with T, and a type
	// starting with V would have had its receiver shadowed by every setter's
	// own `v` parameter.
	taken := map[string]bool{"v": true} // the setter parameter, always present
	for _, f := range u.Fields {
		taken[f.priv] = true
	}
	u.Recv = pick(strings.ToLower(typeName[:1]), taken)
	taken[u.Recv] = true
	// "t" is reserved too: the generated TESTS use CtorLocal as their subject,
	// and it must not shadow *testing.T.
	taken["t"] = true
	u.CtorLocal = pick("out", taken)

	u.Imports = imports(u.Fields)

	if code, err = render(codeTemplate, u); err != nil {
		return nil, nil, err
	}
	if tests, err = render(testTemplate, u); err != nil {
		return nil, nil, err
	}
	return code, tests, nil
}

func render(t *template.Template, u unit) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, u); err != nil {
		return nil, err
	}
	// gofmt the output rather than trusting the template's whitespace: a
	// generator that emits unformatted code makes every diff unreadable.
	out, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated code does not parse (this is a bug in iogen): %w\n%s", err, buf.String())
	}
	return out, nil
}

// exportedIdent turns an IO member name into an exported Go identifier.
// The member name stays the wire contract; this only affects Go source.
func exportedIdent(name string) (string, error) {
	var b strings.Builder
	upper := true
	for _, r := range name {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			upper = true
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upper {
				b.WriteRune(unicode.ToUpper(r))
				upper = false
			} else {
				b.WriteRune(r)
			}
		default:
			return "", fmt.Errorf("cannot be spelled as a Go identifier")
		}
	}
	id := b.String()
	if id == "" || unicode.IsDigit(rune(id[0])) {
		return "", fmt.Errorf("cannot be spelled as a Go identifier")
	}
	return id, nil
}

// pick returns base, or base with enough underscores appended to be unused.
func pick(base string, taken map[string]bool) string {
	if goKeywords[base] {
		base += "_"
	}
	for taken[base] {
		base += "_"
	}
	return base
}

func unexported(id string) string {
	r := []rune(id)
	r[0] = unicode.ToLower(r[0])
	s := string(r)
	if goKeywords[s] {
		return s + "_"
	}
	return s
}

var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

var _ = schema.OpenAny // keep the schema import meaningful if templates change

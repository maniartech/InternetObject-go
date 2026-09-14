package document

import (
	"testing"

	"github.com/maniartech/InternetObject-go/internal/errs"
	"github.com/maniartech/InternetObject-go/internal/parser"
)

func headerOf(t *testing.T, src string) *parser.Header {
	t.Helper()
	doc := parser.Parse(src + "\n---\n")
	if len(doc.Errors) > 0 || doc.Header == nil {
		t.Fatalf("%q: %v", src, doc.Errors)
	}
	return doc.Header
}

// An alias declared ahead of its target resolves to the target's one compiled
// schema, not a second compile of it.
func TestAliasAheadOfItsTargetSharesTheCompiledSchema(t *testing.T) {
	d := NewDefinitions(headerOf(t, "~ $a: $b\n~ $b: {x: int}"))
	a, aerr := d.SchemaOf("a")
	b, berr := d.SchemaOf("$b")
	if aerr != nil || berr != nil || a == nil || a != b {
		t.Fatalf("a=%p (%v), b=%p (%v): want one shared schema", a, aerr, b, berr)
	}
}

// Fault is the first failing definition in HEADER order, although aliases are
// resolved after their targets.
func TestFaultIsTheFirstInHeaderOrder(t *testing.T) {
	d := NewDefinitions(headerOf(t, "~ $a: $nope\n~ $b: {x: nosuchtype}"))
	if f := d.Fault(); f == nil || f.Code != errs.UndefinedSchema {
		t.Fatalf("Fault() = %v, want undefined-schema from $a", f)
	}
	if d := NewDefinitions(headerOf(t, "~ $ok: {x: int}\n~ $alias: $ok")); d.Fault() != nil {
		t.Fatalf("a clean header reports %v", d.Fault())
	}
}

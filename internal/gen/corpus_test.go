package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/maniartech/InternetObject-go/internal/conformance"
)

// The conformance corpus, run through GENERATED code.
//
// Generated types are a third path alongside the tree and the framed one, and
// every path in this library is held to the corpus — a path that is only
// checked by its own hand-written tests is a path that drifts. ADR 0004 D6
// permits an inlined fast path in generated code ONLY behind byte-for-byte
// differential gates; this is that gate, built before the optimisation rather
// than after it.
//
// For every corpus row whose schema iogen accepts, this asserts that
//
//	generated:  v.Unmarshal(input)  then v.Marshal()
//	engine:     io.UnmarshalWith(input, &twin, s) then io.MarshalWith(twin, s)
//
// agree on the error AND byte-for-byte on the output. Today the generated path
// delegates, so agreement is near-tautological — that is exactly the point:
// the gate is in place and green BEFORE anything is inlined, so the day the
// generated writer stops delegating, this test is what catches a divergence.
//
// It compiles a package of ~500 generated types, so it is opt-in, in the same
// way and for the same reason as the 24k round-trip soak:
//
//	IO_GEN_CORPUS=1 go test ./internal/gen/
func TestGeneratedCodeAgainstCorpus(t *testing.T) {
	if os.Getenv("IO_GEN_CORPUS") == "" {
		t.Skip("set IO_GEN_CORPUS=1 to compile and run the generated-code corpus gate")
	}
	cases := collectCases(t)
	if len(cases) < 100 {
		t.Fatalf("only %d generatable corpus cases found; the corpus or the survey is wrong", len(cases))
	}

	dir := t.TempDir()
	writeModule(t, dir)
	for _, c := range cases {
		code, tests, err := Generate("gencorpus", c.Type, c.Schema)
		if err != nil {
			t.Fatalf("%s: schema accepted during collection but not during generation: %v", c.Name, err)
		}
		write(t, filepath.Join(dir, strings.ToLower(c.Type)+".go"), code)
		write(t, filepath.Join(dir, strings.ToLower(c.Type)+"_gen_test.go"), tests)
	}
	write(t, filepath.Join(dir, "corpus_driver_test.go"), []byte(driver(cases)))

	cmd := exec.Command("go", "test", "-count=1", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated code failed the corpus:\n%s", trim(string(out)))
	}
	t.Logf("%d corpus cases ran through generated code, all agreeing with the engine", len(cases))
}

type genCase struct {
	Name   string
	Type   string
	Schema string
	Input  string
}

func collectCases(t *testing.T) []genCase {
	t.Helper()
	dir, err := conformance.CorpusDir()
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, suite := range []string{"validation", "serializer", "document", "schema", "regression", "parser"} {
		fs, _ := filepath.Glob(filepath.Join(dir, suite, "*.io"))
		files = append(files, fs...)
	}
	var out []genCase
	seen := map[string]bool{}
	for _, f := range files {
		rows, err := conformance.LoadIOSuite(f)
		if err != nil {
			continue
		}
		for _, r := range rows {
			schema := r.SchemaDef
			if !r.HasSchemaDef {
				schema = r.Schema
				if !r.HasSchema {
					continue
				}
			}
			schema = strings.TrimSpace(schema)
			if !r.HasInput || schema == "" || seen[schema+"\x00"+r.Input] {
				continue
			}
			name := fmt.Sprintf("T%d", len(out))
			if _, _, err := Generate("gencorpus", name, schema); err != nil {
				continue // iogen declines this shape, loudly and on purpose
			}
			seen[schema+"\x00"+r.Input] = true
			out = append(out, genCase{
				Name:   filepath.Base(f) + " :: " + r.Name,
				Type:   name,
				Schema: schema,
				Input:  r.Input,
			})
		}
	}
	return out
}

// driver emits one test per case. The comparison is written per-case rather
// than through an interface because each case has its own generated type and
// its own plain twin — which is the whole reason this is code generation.
func driver(cases []genCase) string {
	var b strings.Builder
	b.WriteString("// Code generated for the corpus gate. DO NOT EDIT.\n\npackage gencorpus\n\nimport (\n\t\"testing\"\n\n\tio \"github.com/maniartech/InternetObject-go\"\n)\n\n")
	for i, c := range cases {
		priv := strings.ToLower(c.Type)
		fmt.Fprintf(&b, `func TestCorpus%d(t *testing.T) {
	// %s
	const input = %q
	s, err := %sSchemaOf()
	if err != nil {
		t.Fatalf("schema: %%v", err)
	}
	var v %s
	genErr := v.Unmarshal(input)
	var twin %sPlain
	engErr := io.UnmarshalWith(input, &twin, s)
	if (genErr == nil) != (engErr == nil) {
		t.Fatalf("Unmarshal disagrees:\n generated %%v\n engine    %%v", genErr, engErr)
	}
	if genErr != nil {
		if genErr.Error() != engErr.Error() {
			t.Fatalf("error text disagrees:\n generated %%q\n engine    %%q", genErr, engErr)
		}
		return
	}
	got, gotErr := v.Marshal()
	want, wantErr := io.MarshalWith(twin, s)
	if (gotErr == nil) != (wantErr == nil) {
		t.Fatalf("Marshal disagrees:\n generated %%v\n engine    %%v", gotErr, wantErr)
	}
	if gotErr == nil && got != want {
		t.Fatalf("output differs:\n generated %%q\n engine    %%q", got, want)
	}
}

`, i, c.Name, c.Input, c.Type, c.Type, priv)
	}
	return b.String()
}

func writeModule(t *testing.T, dir string) {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repo := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	gomod := fmt.Sprintf("module gencorpus\n\ngo 1.24\n\nrequire github.com/maniartech/InternetObject-go v0.0.0\n\nreplace github.com/maniartech/InternetObject-go => %s\n",
		filepath.ToSlash(repo))
	write(t, filepath.Join(dir, "go.mod"), []byte(gomod))
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func trim(s string) string {
	if len(s) > 6000 {
		return s[:6000] + "\n… (truncated)"
	}
	return s
}

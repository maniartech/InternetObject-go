package internetobject_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The io-playground samples, as a second corpus.
//
// io-test-cases gates CONFORMANCE — the behaviours every implementation must
// share. The playground samples gate something different and equally real:
// the documents the format SHOWS PEOPLE. They are hand-picked to demonstrate
// what Internet Object is for, so a sample this port cannot read is a document
// a reader will copy out of the playground and find broken.
//
// They earned their place immediately. Running them found two library bugs the
// full corpus never caught, because no corpus case covers either: an array of a
// schema reference (`books:[$B]`), which the flagship Multiple Sections sample
// uses, and fractional seconds beyond three digits.
//
// This test pins the EXACT state of every sample. A sample that starts passing
// fails this test just as loudly as one that regresses — that is deliberate:
// a known gap being fixed is news, and news belongs in the table below rather
// than silently absorbed.

const (
	samplePasses = iota // must parse with no error
	sampleErrors        // an intentional-error sample: must report errors
	sampleGap           // a known io-go gap: must STILL fail, with the reason
)

// Every sample, and what it does today. The reason on a gap is the finding.
var playgroundExpect = map[string]struct {
	state  int
	reason string
}{
	// Simple
	"simple-object":                 {samplePasses, ""},
	"simple-collection":             {samplePasses, ""},
	"typed-collection":              {samplePasses, ""},
	"simple-collection-with-errors": {sampleErrors, "the sample exists to show error recovery"},

	// Schema and definition
	"employee-register":        {samplePasses, ""},
	"recursive-schema":         {samplePasses, ""},
	"recursive-schema-comples": {samplePasses, ""},

	// IO types
	"any":       {samplePasses, ""},
	"strings":   {samplePasses, ""},
	"numbers":   {samplePasses, ""},
	"datetimes": {samplePasses, ""},
	"arrays":    {samplePasses, ""},
	"objects":   {samplePasses, ""},

	// Multiple sections - the format's headline capability
	"multiple-sections": {samplePasses, ""},

	// JSON
	"json":        {samplePasses, ""},
	"json-schema": {sampleErrors, "the sample is titled 'Intentional Error'"},

	// Advanced and complex
	"complex-library": {samplePasses, ""},
	"variables":       {samplePasses, ""},
	"separate-schema": {sampleGap, "the sample keeps DEFINITIONS in one panel and a " +
		"document WITH ITS OWN HEADER in the other; io-go has no way to parse a " +
		"document against preloaded definitions, so the two headers cannot be joined. " +
		"Stream already takes StreamOptions.Definitions; Parse has no equivalent"},

	// Applications and use cases
	"api-multiple-collections-response": {samplePasses, ""},
	"app-seed-data":                     {samplePasses, ""},
	"ml-training-data":                  {samplePasses, ""},
	"structured-logging":                {samplePasses, ""},
	"api-collection-response":           {sampleGap, "joining a bare schema panel to the document is not yet right; needs the definitions route above"},
	"as-config":                         {sampleGap, "positional defaults in a typedef, e.g. {bool, F} and {int, 8000}"},
}

func TestPlaygroundSamples(t *testing.T) {
	dir := playgroundDir(t)
	files := playgroundFiles(t, dir)
	if len(files) != len(playgroundExpect) {
		t.Errorf("found %d samples but the table lists %d; add or remove entries",
			len(files), len(playgroundExpect))
	}

	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".ts")
		want, known := playgroundExpect[name]
		if !known {
			t.Errorf("%s: new playground sample, not in the table", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			src, ok := playgroundSource(t, f)
			if !ok {
				t.Fatalf("no doc literal in %s", f)
			}
			_, err := io.Parse(src)
			switch want.state {
			case samplePasses:
				if err != nil {
					t.Fatalf("sample no longer parses: %v", err)
				}
			case sampleErrors:
				if err == nil {
					t.Fatalf("an intentional-error sample parsed clean (%s)", want.reason)
				}
			case sampleGap:
				if err == nil {
					t.Fatalf("KNOWN GAP IS FIXED - promote this sample to samplePasses."+nl+"  gap was: %s", want.reason)
				}
			}
		})
	}
}

func playgroundDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("IO_PLAYGROUND_DIR"); d != "" {
		return d
	}
	_, thisFile, _, _ := runtime.Caller(0)
	repo := filepath.Dir(thisFile)
	dir := filepath.Join(filepath.Dir(repo), "io-playground", "src", "sample-data")
	if _, err := os.Stat(dir); err != nil {
		// Same discipline as the conformance corpus: a missing gate FAILS
		// rather than skipping, so it cannot rot unnoticed.
		t.Fatalf("playground samples not found at %s (set IO_PLAYGROUND_DIR): %v", dir, err)
	}
	return dir
}

func playgroundFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".ts") {
			return nil
		}
		switch filepath.Base(p) {
		case "index.ts", "options.ts", "sample-options.ts":
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

var reSampleLit = regexp.MustCompile(`(?s)const\s+(schema|doc)\s*=\s*` + "`" + `(.*?)` + "`")

// playgroundSource rebuilds one sample as a single document.
//
// The playground itself does NOT do this: it parses the schema panel as
// definitions and hands them to the document parse. io-go has no Parse-level
// equivalent yet (only StreamOptions.Definitions), so joining is the closest
// available, and the samples where it is not equivalent are the gaps above.
func playgroundSource(t *testing.T, file string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var schema, doc string
	for _, m := range reSampleLit.FindAllStringSubmatch(string(b), -1) {
		if m[1] == "schema" {
			schema = strings.TrimSpace(unescapeTemplate(m[2]))
		} else {
			doc = strings.TrimSpace(unescapeTemplate(m[2]))
		}
	}
	if doc == "" {
		return "", false
	}
	if schema == "" {
		return doc, true
	}
	sep := nl + "---" + nl
	if strings.HasPrefix(doc, "---") {
		sep = nl
	}
	return schema + sep + doc, true
}

// unescapeTemplate resolves the escapes a TypeScript TEMPLATE LITERAL applies,
// which the raw file text still carries. Without it a sample pattern reaches
// the parser with a doubled backslash and "fails" for a reason that is entirely
// this harness's fault - which is exactly what happened the first time.
func unescapeTemplate(s string) string {
	const backslash = 92
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != backslash || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte(10)
		case 't':
			b.WriteByte(9)
		case 'r':
			b.WriteByte(13)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// nl is a newline without writing an escape sequence in this source.
var nl = string(rune(10))

var _ = fmt.Sprintf

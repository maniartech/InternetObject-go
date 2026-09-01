package conformance

import (
	"path/filepath"
	"testing"
)

// TestParserSuite is the phase-2 gate: every case in the parser/ suite, read
// self-hosted — the suite files themselves are parsed by the parser under
// test. Valid (value-asserting) and invalid (code-asserting) cases are
// reported separately: an implementation that rejects everything passes every
// invalid case, and a single combined percentage would hide that.
func TestParserSuite(t *testing.T) {
	runIOSuiteDir(t, "parser")
}

// TestSchemaSuite is the phase-3 gate: schema definition strings compiled and
// compared by subset against the neutral shape.
func TestSchemaSuite(t *testing.T) {
	runIOSuiteDir(t, "schema")
}

// TestValidationSuite is the phase-4 gate: schema + data → validated value or
// designated codes.
func TestValidationSuite(t *testing.T) {
	runIOSuiteDir(t, "validation")
}

// TestSerializerSuite is the phase-5 gate: canonical output, value
// preservation, and idempotence, per case.
func TestSerializerSuite(t *testing.T) {
	runIOSuiteDir(t, "serializer")
}

// TestDocumentSuite is the phase-6 gate: sections, header, core types.
func TestDocumentSuite(t *testing.T) {
	runIOSuiteDir(t, "document")
}

// TestRegressionSuite is the phase-8 gate: every bug the reference ever had,
// pinned. Passing these first time is evidence the corpus generalizes.
func TestRegressionSuite(t *testing.T) {
	runIOSuiteDir(t, "regression")
}

func runIOSuiteDir(t *testing.T, dir string) {
	root, err := CorpusDir()
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, dir, "*.io"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no suite files under %s/%s — the gate did not run", root, dir)
	}
	if sub, _ := filepath.Glob(filepath.Join(root, dir, "*", "*.io")); len(sub) > 0 {
		files = append(files, sub...) // e.g. regression/fixed/
	}

	const detailLimit = 20
	validPass, validFail, invalidPass, invalidFail, inert := 0, 0, 0, 0, 0
	shown := 0
	for _, file := range files {
		rows, err := LoadIOSuite(file)
		if err != nil {
			t.Errorf("FAIL %s", err)
			validFail++ // a suite that cannot load counts against the gate
			continue
		}
		for _, row := range rows {
			var problems []string
			switch {
			case dir == "schema":
				if !row.HasSchemaDef {
					inert++
					continue
				}
				problems = RunSchemaDefCase(row)
			case dir == "validation":
				if !row.HasInput || !row.HasSchema {
					inert++
					continue
				}
				problems = RunValidationCase(row)
			case dir == "serializer":
				if !row.HasInput || !row.HasOutput {
					inert++
					continue
				}
				problems = RunRoundtripCase(row)
			default:
				if !row.HasInput {
					inert++
					continue
				}
				problems = RunParseCase(row)
			}
			invalid := len(row.ErrorCodes) > 0
			if len(problems) == 0 {
				if invalid {
					invalidPass++
				} else {
					validPass++
				}
				continue
			}
			if invalid {
				invalidFail++
			} else {
				validFail++
			}
			if shown++; shown <= detailLimit {
				t.Errorf("FAIL %s :: %s  input=%q", filepath.Base(file), row.Name, row.Input)
				for _, p := range problems {
					t.Errorf("   %s", p)
				}
			}
		}
	}
	if shown > detailLimit {
		t.Errorf("(… %d more failing cases not detailed)", shown-detailLimit)
	}

	total := validPass + validFail + invalidPass + invalidFail
	if total == 0 {
		t.Fatalf("%s: zero cases ran — the gate did not run", dir)
	}
	t.Logf("%s/: valid %d/%d, invalid %d/%d (%d cases, %d inert) — %s",
		dir, validPass, validPass+validFail, invalidPass, invalidPass+invalidFail,
		total, inert, PinLine(root))
	if validFail+invalidFail > 0 {
		t.Fail()
	}
}

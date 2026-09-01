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

func runIOSuiteDir(t *testing.T, dir string) {
	root, err := CorpusDir()
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, dir, "*.io"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no suite files under %s/%s — the gate did not run", root, dir)
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
			if !row.HasInput {
				inert++
				continue
			}
			problems := RunParseCase(row)
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

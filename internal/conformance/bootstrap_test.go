package conformance

import (
	"path/filepath"
	"testing"
)

// TestBootstrapTokenizer is the phase-1 gate: every case in
// bootstrap/tokenizer.csv, against this tokenizer. A missing corpus or an
// empty case list FAILS — the gate must be seen to run.
func TestBootstrapTokenizer(t *testing.T) {
	dir, err := CorpusDir()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "bootstrap", "tokenizer.csv")
	cases, err := LoadBootstrapCases(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s: no cases loaded — the gate did not run", file)
	}

	const detailLimit = 15 // full per-case detail for the first N failures
	pass, fail := 0, 0
	for _, c := range cases {
		problems := RunTokenCase(c)
		if len(problems) == 0 {
			pass++
			continue
		}
		fail++
		if fail <= detailLimit {
			t.Errorf("FAIL %s/%s  input=%q", c.Suite, c.Name, c.Input)
			for _, p := range problems {
				t.Errorf("   %s", p)
			}
		}
	}
	if fail > detailLimit {
		t.Errorf("(… %d more failing cases not detailed)", fail-detailLimit)
	}

	t.Logf("bootstrap/tokenizer.csv: %d passed, %d failed (%d cases) — %s",
		pass, fail, len(cases), PinLine(dir))
	if fail > 0 {
		t.Fail()
	}
}

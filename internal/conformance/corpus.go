// Package conformance runs the shared Internet Object conformance corpus
// (io-test-cases) against this implementation.
//
// The corpus is the definition of done. It is located as a sibling checkout of
// this repository (../io-test-cases), overridable with the IO_CORPUS_DIR
// environment variable. A missing corpus FAILS the suite — it never skips.
package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CorpusPin is the io-test-cases commit this implementation claims its numbers
// against. The corpus has no version tags yet (upstream ADR 0007 D5); when it
// gains one, this pin moves to the tag. A conformance number with no version on
// it is a rumour, so every report prints this pin — and warns when the sibling
// checkout's actual HEAD differs.
const CorpusPin = "15e02ceb9b240ccb554ef4957adcd6ee619fbf50"

// CorpusDir returns the root of the io-test-cases checkout, or an error when it
// cannot be found. Resolution order: IO_CORPUS_DIR, then the sibling directory
// ../io-test-cases relative to this repository.
func CorpusDir() (string, error) {
	if dir := os.Getenv("IO_CORPUS_DIR"); dir != "" {
		if _, err := os.Stat(dir); err != nil {
			return "", fmt.Errorf("IO_CORPUS_DIR points at %q, which is not readable: %w", dir, err)
		}
		return dir, nil
	}
	// This file lives at <repo>/internal/conformance/corpus.go; the corpus is a
	// sibling of <repo>.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate this source file to resolve the sibling corpus")
	}
	repo := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	dir := filepath.Join(filepath.Dir(repo), "io-test-cases")
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("corpus not found at %s (set IO_CORPUS_DIR to a checkout of io-test-cases): %w", dir, err)
	}
	return dir, nil
}

// CorpusHead best-effort reads the checkout's current commit hash so a report
// can warn when it drifts from CorpusPin. Returns "" when it cannot be read
// (e.g. a non-git export); that is not an error.
func CorpusHead(dir string) string {
	head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(head))
	if hash, found := strings.CutPrefix(ref, "ref: "); found {
		b, err := os.ReadFile(filepath.Join(dir, ".git", filepath.FromSlash(hash)))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	return ref
}

// PinLine is the version line printed with every conformance report.
func PinLine(dir string) string {
	line := "corpus " + CorpusPin[:12]
	if head := CorpusHead(dir); head != "" && head != CorpusPin {
		line += fmt.Sprintf(" — WARNING: checkout is at %s, which is NOT the pinned commit", head[:12])
	}
	return line
}

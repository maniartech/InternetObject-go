// Command iogen generates a guarded Go type from an Internet Object schema.
//
// The schema file is the source of truth (ADR 0004 D6). The generated type
// keeps its fields unexported and exposes a constructor, getters and typed
// setters, so a value that exists is one the schema accepted — the guarantee a
// tagged struct cannot give, because nothing stops `u.Age = -1` there.
//
// Usage, normally from a //go:generate line:
//
//	//go:generate go run github.com/maniartech/InternetObject-go/cmd/iogen -schema user.io -type User
//
//	iogen -schema user.io -type User [-out user_gen.go] [-package pkg]
//
// It writes <out> and <out with _gen.go replaced by _gen_test.go>. The tests
// are part of the contract, not a convenience: they hold the generated code to
// the engine's own behaviour, which is what makes "zero semantic logic" a
// checkable claim rather than an intention.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	var (
		schemaPath = flag.String("schema", "", "path to the .io schema file (required)")
		typeName   = flag.String("type", "", "name of the Go type to generate (required)")
		outPath    = flag.String("out", "", "output file (default: <type lowercased>_gen.go)")
		pkgName    = flag.String("package", "", "package name (default: the output directory's)")
	)
	flag.Parse()

	if err := run(*schemaPath, *typeName, *outPath, *pkgName); err != nil {
		fmt.Fprintln(os.Stderr, "iogen:", err)
		os.Exit(1)
	}
}

func run(schemaPath, typeName, outPath, pkgName string) error {
	if schemaPath == "" || typeName == "" {
		flag.Usage()
		return fmt.Errorf("-schema and -type are both required")
	}
	src, err := os.ReadFile(schemaPath)
	if err != nil {
		return err
	}
	if outPath == "" {
		outPath = strings.ToLower(typeName) + "_gen.go"
	}
	if pkgName == "" {
		if pkgName, err = packageOf(outPath); err != nil {
			return err
		}
	}

	code, tests, err := Generate(pkgName, typeName, strings.TrimSpace(string(src)))
	if err != nil {
		return fmt.Errorf("%s: %w", schemaPath, err)
	}
	if err := os.WriteFile(outPath, code, 0o644); err != nil {
		return err
	}
	testPath := strings.TrimSuffix(outPath, ".go") + "_test.go"
	if err := os.WriteFile(testPath, tests, 0o644); err != nil {
		return err
	}
	fmt.Printf("iogen: wrote %s and %s\n", outPath, testPath)
	return nil
}

// packageOf reads the package name from a Go file already in the output
// directory, so a generated file lands in the package that is actually there
// rather than one guessed from the directory name — those differ often enough
// (v2, cmd/x, internal/foo-bar) that guessing would be wrong quietly.
func packageOf(outPath string) (string, error) {
	dir := filepath.Dir(outPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if filepath.Clean(filepath.Join(dir, name)) == filepath.Clean(outPath) {
			continue // do not read the file we are about to overwrite
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if pkg, ok := strings.CutPrefix(strings.TrimSpace(line), "package "); ok {
				return strings.TrimSpace(pkg), nil
			}
		}
	}
	return "", fmt.Errorf("cannot determine the package for %s: no other .go file in %s; pass -package", outPath, dir)
}

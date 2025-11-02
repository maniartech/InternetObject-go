//go:build zerorunner
// +build zerorunner

package main

import (
	"fmt"

	"github.com/maniartech/InternetObject-go/parsers"
	"github.com/maniartech/InternetObject-go/utils"
)

func main() {
	// Test Internet Object parsing with ZeroParser
	// Test with open strings containing spaces
	testInputs := []string{
		// Full document with header and data section
		`
		name, address, tags
		---
		John Doe, {123 Main St, New York, state: NY}, [tag1, tag2]
		`,
	}

	for i, ioStr := range testInputs {
		fmt.Printf("\n=== Test %d ===\n", i+1)
		fmt.Println("Input:", ioStr)
		fmt.Println()

		ast, err := parsers.ParseString(ioStr)
		if err != nil {
			fmt.Println("❌ Parse error:", err)
		}

		utils.PrettyPrint(ast)
	}
}

package main

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/parsers"
)

// printAST recursively prints the AST structure
func printAST(parser *parsers.FastParserBytes, valueIdx int, indent int) {
	value := parser.GetValue(valueIdx)
	prefix := strings.Repeat("  ", indent)

	switch value.Type {
	case parsers.TypeObject:
		fmt.Printf("%s{\n", prefix)
		for i := 0; i < value.ChildCount; i++ {
			member := parser.GetMember(value.FirstChild + i)
			key := parser.GetMemberKey(member)
			fmt.Printf("%s  \"%s\": ", prefix, key)
			printAST(parser, member.ValueIdx, indent+1)
		}
		fmt.Printf("%s}\n", prefix)

	case parsers.TypeArray:
		fmt.Printf("[\n")
		for i := 0; i < value.ChildCount; i++ {
			fmt.Printf("%s  ", prefix)
			printAST(parser, value.FirstChild+i, indent+1)
		}
		fmt.Printf("%s]\n", prefix)

	case parsers.TypeString:
		str := parser.GetString(value)
		fmt.Printf("\"%s\"\n", str)

	case parsers.TypeInt:
		fmt.Printf("%d\n", value.IntValue)

	case parsers.TypeFloat:
		fmt.Printf("%f\n", value.FloatValue)

	case parsers.TypeBool:
		fmt.Printf("%v\n", value.BoolValue)

	case parsers.TypeNull:
		fmt.Printf("null\n")

	default:
		fmt.Printf("unknown\n")
	}
}

func main() {
	// Example with validation errors - uncomment to test different errors

	// Valid JSON
	/*
		jsonStr := `{
			"name": "John Doe",
			"age": 30,
			"message": "Hello\nWorld",
			"active": true,
			"scores": [98, 87, 92],
			"nested": {
				"city": "New York",
				"country": "USA"
			}
		}`
	*/

	// Test error cases (uncomment one to test):

	// 1. Invalid UTF-8
	// jsonStr := `{"text": "Invalid \xC3"}`

	// 2. Number overflow
	// jsonStr := `{"bigNum": 99999999999999999999}`

	// 3. Duplicate keys
	jsonStr := `
	name, age, gender
	---
	John, 30, male
	`
	// jsonStr = `{"a": 1} extra`

	// 5. Invalid escape sequence
	// jsonStr = `{"text": "Hello\xWorld"}`

	// 6. Incomplete unicode escape
	// jsonStr = `{"text": "\u12"}`

	// Create parser from string
	parser := parsers.NewFastParserBytesFromString(jsonStr, 1024)

	// Parse the JSON
	rootIdx, err := parser.Parse()
	if err != nil {
		fmt.Printf("❌ Parse error: %v\n\n", err)
		showErrorContext(jsonStr)
		return
	}

	// Print the entire AST
	fmt.Println("✅ Parse successful!")
	fmt.Println("\n=== AST Structure ===")
	printAST(parser, rootIdx, 0)
}

// showErrorContext shows the input with line numbers for debugging
func showErrorContext(input string) {
	lines := strings.Split(input, "\n")

	fmt.Println("Input with line numbers:")
	fmt.Println(strings.Repeat("=", 60))
	for i, line := range lines {
		lineNum := i + 1
		fmt.Printf("%4d | %s\n", lineNum, line)
	}
	fmt.Println(strings.Repeat("=", 60))
}

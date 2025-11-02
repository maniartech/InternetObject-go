package main

import (
	"fmt"
	"strings"

	"github.com/maniartech/InternetObject-go/parsers"
)

// printDetailedAST prints complete AST details with all information
func printDetailedAST(parser *parsers.ZeroParser, nodeIdx uint32, indent int, label string) {
	prefix := strings.Repeat("│  ", indent)

	if nodeIdx == 0xFFFFFFFF {
		fmt.Printf("%s└─ %s: NULL\n", prefix, label)
		return
	}

	node := parser.GetNode(nodeIdx)

	// Print node header with all details
	fmt.Printf("%s┌─ %s [Node#%d]\n", prefix, label, nodeIdx)
	fmt.Printf("%s│  Type: %s\n", prefix, getNodeTypeName(node.Type))
	fmt.Printf("%s│  Position: Row %d, Col %d\n", prefix, node.Row, node.Col)
	fmt.Printf("%s│  TokenIdx: %d\n", prefix, node.TokenIdx)
	fmt.Printf("%s│  ChildCount: %d\n", prefix, node.ChildCount)

	// Decode and display node flags
	nodeFlagNames := getNodeFlagNames(node.Flags)
	if len(nodeFlagNames) > 0 {
		fmt.Printf("%s│  Flags: [%s]\n", prefix, strings.Join(nodeFlagNames, ", "))
	} else {
		fmt.Printf("%s│  Flags: None\n", prefix)
	}

	// Print token details if present
	if node.TokenIdx != 0xFFFFFFFF {
		token := parser.GetToken(node.TokenIdx)
		tokenValue := parser.GetTokenString(node.TokenIdx)
		fmt.Printf("%s│  Token: Type=%s, SubType=%s\n", prefix, getTokenTypeName(token.Type), getSubTypeName(token.SubType))
		fmt.Printf("%s│  Token Position: [%d:%d] Row %d, Col %d\n", prefix, token.Start, token.End, token.Row, token.Col)

		// Decode and display token flags
		flagNames := getTokenFlagNames(token.Flags)
		if len(flagNames) > 0 {
			fmt.Printf("%s│  Token Flags: [%s]\n", prefix, strings.Join(flagNames, ", "))
		} else {
			fmt.Printf("%s│  Token Flags: None\n", prefix)
		}

		fmt.Printf("%s│  Token Value: \"%s\"\n", prefix, tokenValue)
	}

	// Print children
	children := parser.GetNodeChildren(nodeIdx)
	if len(children) > 0 {
		fmt.Printf("%s│  Children (%d):\n", prefix, len(children))
		for i, childIdx := range children {
			childLabel := fmt.Sprintf("Child[%d]", i)

			// Add semantic labels for specific node types
			childNode := parser.GetNode(childIdx)
			switch node.Type {
			case parsers.NodeKindMember:
				if i == 0 {
					childLabel = "Value"
				}
			case parsers.NodeKindObject, parsers.NodeKindArray:
				if childNode.Type == parsers.NodeKindMember {
					keyStr := parser.GetTokenString(childNode.TokenIdx)
					childLabel = fmt.Sprintf("Member[%d] key=\"%s\"", i, keyStr)
				} else {
					childLabel = fmt.Sprintf("Element[%d]", i)
				}
			case parsers.NodeKindDocument:
				childLabel = fmt.Sprintf("Section[%d]", i)
			}

			printDetailedAST(parser, childIdx, indent+1, childLabel)
		}
	}

	fmt.Printf("%s└─ End of Node#%d\n", prefix, nodeIdx)
}

func getNodeTypeName(nodeType uint8) string {
	switch nodeType {
	case parsers.NodeKindDocument:
		return "Document"
	case parsers.NodeKindSection:
		return "Section"
	case parsers.NodeKindCollection:
		return "Collection"
	case parsers.NodeKindObject:
		return "Object"
	case parsers.NodeKindArray:
		return "Array"
	case parsers.NodeKindMember:
		return "Member"
	case parsers.NodeKindToken:
		return "Token"
	case parsers.NodeKindError:
		return "Error"
	default:
		return fmt.Sprintf("Unknown(%d)", nodeType)
	}
}

func getTokenTypeName(tokType uint8) string {
	switch tokType {
	case parsers.TokInvalid:
		return "Invalid"
	case parsers.TokString:
		return "String"
	case parsers.TokNumber:
		return "Number"
	case parsers.TokBoolean:
		return "Boolean"
	case parsers.TokNull:
		return "Null"
	case parsers.TokBigInt:
		return "BigInt"
	case parsers.TokDecimal:
		return "Decimal"
	case parsers.TokBinary:
		return "Binary"
	case parsers.TokDateTime:
		return "DateTime"
	case parsers.TokCurlyOpen:
		return "CurlyOpen"
	case parsers.TokCurlyClose:
		return "CurlyClose"
	case parsers.TokBracketOpen:
		return "BracketOpen"
	case parsers.TokBracketClose:
		return "BracketClose"
	case parsers.TokColon:
		return "Colon"
	case parsers.TokComma:
		return "Comma"
	case parsers.TokTilde:
		return "Tilde"
	case parsers.TokSectionSep:
		return "SectionSep"
	default:
		return fmt.Sprintf("Unknown(%d)", tokType)
	}
}

func getSubTypeName(subType uint8) string {
	switch subType {
	case parsers.SubTypeNone:
		return "None"
	case parsers.SubTypeOpenString:
		return "OpenString"
	case parsers.SubTypeRegularString:
		return "RegularString"
	case parsers.SubTypeRawString:
		return "RawString"
	case parsers.SubTypeSectionName:
		return "SectionName"
	case parsers.SubTypeSectionSchema:
		return "SectionSchema"
	default:
		return fmt.Sprintf("Unknown(%d)", subType)
	}
}

func getTokenFlagNames(flags uint8) []string {
	var names []string

	if flags&parsers.FlagHasEscapes != 0 {
		names = append(names, "HasEscapes")
	}
	if flags&parsers.FlagNeedsNormalize != 0 {
		names = append(names, "NeedsNormalize")
	}
	if flags&parsers.FlagIsHex != 0 {
		names = append(names, "IsHex")
	}
	if flags&parsers.FlagIsOctal != 0 {
		names = append(names, "IsOctal")
	}
	if flags&parsers.FlagIsBinary != 0 {
		names = append(names, "IsBinary")
	}
	if flags&parsers.FlagIsNegative != 0 {
		names = append(names, "IsNegative")
	}
	if flags&parsers.FlagHasDecimal != 0 {
		names = append(names, "HasDecimal")
	}
	if flags&parsers.FlagHasExponent != 0 {
		names = append(names, "HasExponent")
	}

	return names
}

func getNodeFlagNames(flags uint8) []string {
	var names []string

	if flags&parsers.NodeFlagIsOpen != 0 {
		names = append(names, "IsOpen")
	}
	if flags&parsers.NodeFlagHasError != 0 {
		names = append(names, "HasError")
	}
	if flags&parsers.NodeFlagHasSchema != 0 {
		names = append(names, "HasSchema")
	}
	if flags&parsers.NodeFlagHasName != 0 {
		names = append(names, "HasName")
	}
	if flags&parsers.NodeFlagHasKey != 0 {
		names = append(names, "HasKey")
	}

	return names
}

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
		// Only closed object
		`{123 Main St, New York, state: NY}`,
		// Only array
		`[tag1, tag2]`,
	}

	for i, ioStr := range testInputs {
		fmt.Printf("\n=== Test %d ===\n", i+1)
		fmt.Println("Input:", ioStr)
		fmt.Println()

		// Create ZeroParser
		parser := parsers.NewZeroParser(ioStr)

		// Parse the Internet Object
		rootIdx, err := parser.Parse()
		if err != nil {
			fmt.Printf("❌ Parse error: %v\n\n", err)
			showErrorContext(ioStr)
			continue
		}

		// Print statistics
		fmt.Println("✅ Parse successful!")
		fmt.Printf("📊 Tokens: %d, Nodes: %d\n", parser.GetTokenCount(), parser.GetNodeCount())

		// Print the entire AST with complete details
		fmt.Println("\n=== AST Tree ===")
		printDetailedAST(parser, rootIdx, 0, "ROOT")
		fmt.Println()
	}
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

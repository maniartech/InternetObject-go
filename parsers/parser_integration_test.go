package parsers_test

import (
	"testing"

	"github.com/maniartech/InternetObject-go/parsers"
)

// Integration tests use the public API from outside the package (black box testing)

// TestParseString_RealWorldUserProfile tests parsing a realistic user profile document
func TestParseString_RealWorldUserProfile(t *testing.T) {
	input := `
name: "Alice Johnson", email: "alice@example.com", verified: true
--- profile
{
	bio: "Software engineer and open source enthusiast",
	location: "San Francisco, CA",
	joined: dt"2023-01-15T10:30:00Z",
	followers: 1542,
	following: 328
}
--- repositories
~ {name: "awesome-go", stars: 234, language: "Go", private: false}
~ {name: "web-framework", stars: 89, language: "Go", private: false}
~ {name: "personal-site", stars: 12, language: "JavaScript", private: true}
`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse user profile: %v", err)
	}

	// Verify header
	if doc.Header == nil {
		t.Fatal("Expected header section")
	}

	// Verify sections
	if len(doc.Sections) != 2 {
		t.Errorf("Expected 2 sections, got %d", len(doc.Sections))
	}
}

// TestParseString_RealWorldEcommerceOrder tests parsing an e-commerce order document
func TestParseString_RealWorldEcommerceOrder(t *testing.T) {
	input := `
orderId: "ORD-2024-001", customerId: "CUST-5678", orderDate: dt"2024-10-29T14:30:00Z"
--- billing
{
	name: "John Doe",
	address: {
		street: "123 Main St",
		city: "Boston",
		state: "MA",
		zip: "02101"
	},
	card: {type: "VISA", last4: "4242"}
}
--- items
~ {sku: "PROD-001", name: "Wireless Mouse", qty: 2, price: 29.99}
~ {sku: "PROD-042", name: "USB-C Cable", qty: 3, price: 12.99}
~ {sku: "PROD-128", name: "Laptop Stand", qty: 1, price: 45.00}
--- totals
{subtotal: 148.94, tax: 11.92, shipping: 5.99, total: 166.85}
`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse order: %v", err)
	}

	if doc.Header == nil {
		t.Fatal("Expected header section")
	}

	if len(doc.Sections) != 3 {
		t.Errorf("Expected 3 sections, got %d", len(doc.Sections))
	}
}

// TestParseString_RealWorldConfigFile tests parsing a configuration file
func TestParseString_RealWorldConfigFile(t *testing.T) {
	input := `
version: "2.1", environment: "production", debug: false
--- database
{
	host: "db.example.com",
	port: 5432,
	name: "myapp_prod",
	maxConnections: 100,
	timeout: 30,
	ssl: true
}
--- cache
{
	enabled: true,
	provider: "redis",
	servers: ["cache1.example.com:6379", "cache2.example.com:6379"],
	ttl: 3600
}
--- logging
{
	level: "info",
	outputs: ["stdout", "file"],
	file: {path: "/var/log/app.log", maxSize: 100, maxBackups: 5}
}
`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if doc.Header == nil {
		t.Fatal("Expected header")
	}

	if len(doc.Sections) != 3 {
		t.Errorf("Expected 3 sections, got %d", len(doc.Sections))
	}
}

// TestParseString_RealWorldAPIResponse tests parsing an API response
func TestParseString_RealWorldAPIResponse(t *testing.T) {
	input := `
status: 200, timestamp: dt"2024-10-29T15:45:00Z", requestId: "req-abc123"
--- data
~ {id: 1, username: "alice", active: true, lastLogin: dt"2024-10-28T10:30:00Z"}
~ {id: 2, username: "bob", active: true, lastLogin: dt"2024-10-29T08:15:00Z"}
~ {id: 3, username: "charlie", active: false, lastLogin: dt"2024-10-20T14:20:00Z"}
--- pagination
{page: 1, pageSize: 3, totalPages: 10, totalRecords: 28}
`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse API response: %v", err)
	}

	if doc.Header == nil {
		t.Fatal("Expected header")
	}

	if len(doc.Sections) != 2 {
		t.Errorf("Expected 2 sections, got %d", len(doc.Sections))
	}
}

// TestParseString_EmptyVariations tests various empty/minimal documents
func TestParseString_EmptyVariations(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"only whitespace", "   \n  \t  \n  "},
		{"only comment", "# This is just a comment"},
		{"single value", "42"},
		{"single string", `"hello"`},
		{"single boolean", "true"},
		{"single null", "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parsers.ParseString(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse %q: %v", tt.name, err)
			}
			if doc == nil {
				t.Fatal("Expected non-nil document")
			}
		})
	}
}

// TestParseString_DeeplyNestedStructures tests handling of deep nesting
func TestParseString_DeeplyNestedStructures(t *testing.T) {
	input := `{
		level1: {
			level2: {
				level3: {
					level4: {
						level5: {
							value: "deep",
							data: [1, 2, 3, [4, 5, [6, 7]]]
						}
					}
				}
			}
		}
	}`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse deeply nested structure: %v", err)
	}

	if doc == nil {
		t.Fatal("Expected non-nil document")
	}
}

// TestParseString_MixedCollectionTypes tests collections with varied content
func TestParseString_MixedCollectionTypes(t *testing.T) {
	input := `
--- mixed
~ {type: "object", data: {a: 1, b: 2}}
~ {type: "array", data: [1, 2, 3, 4, 5]}
~ {type: "nested", data: {obj: {arr: [1, 2]}, val: 42}}
~ {type: "simple", value: "test"}
`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse mixed collections: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Errorf("Expected 1 section, got %d", len(doc.Sections))
	}
}

// TestParseString_LargeDocument tests handling of larger documents
func TestParseString_LargeDocument(t *testing.T) {
	// Build a document with many sections and items
	input := `version: "1.0", generated: dt"2024-10-29T00:00:00Z"
`

	// Add 25 sections with collections (unique names using letters A-Y)
	for i := 0; i < 25; i++ {
		input += "--- section" + string(rune('A'+i)) + "\n"
		input += "~ {id: " + string(rune('0'+i%10)) + ", name: \"Item\", value: 100}\n"
		input += "~ {id: " + string(rune('0'+(i+1)%10)) + ", name: \"Item\", value: 200}\n"
	}

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse large document: %v", err)
	}

	if doc.Header == nil {
		t.Fatal("Expected header")
	}

	if len(doc.Sections) != 25 {
		t.Errorf("Expected 25 sections, got %d", len(doc.Sections))
	}
}

// TestParseString_SpecialCharactersInStrings tests string handling
func TestParseString_SpecialCharactersInStrings(t *testing.T) {
	input := `{
		escaped: "Line 1\nLine 2\tTabbed",
		unicode: "Hello 🌍 World",
		quotes: "He said \"Hello\"",
		url: "https://example.com/path?query=value&foo=bar"
	}`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse special characters: %v", err)
	}

	if doc == nil {
		t.Fatal("Expected non-nil document")
	}
}

// TestParseString_NumericVariations tests various number formats
func TestParseString_NumericVariations(t *testing.T) {
	input := `{
		integer: 42,
		negative: -123,
		float: 3.14159,
		scientific: 1.23e10,
		hex: 0xFF,
		octal: 0o77,
		binary: 0b1010,
		infinity: Infinity,
		negInfinity: -Infinity,
		notANumber: NaN
	}`

	doc, err := parsers.ParseString(input)
	if err != nil {
		t.Fatalf("Failed to parse numeric variations: %v", err)
	}

	if doc == nil {
		t.Fatal("Expected non-nil document")
	}
}

// TestParseString_OpenObjectVariations tests open object syntax
func TestParseString_OpenObjectVariations(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple open", `a: 1, b: 2, c: 3`},
		{"mixed types", `name: "test", age: 30, active: true, score: null`},
		{"nested in open", `outer: {inner: {value: 42}}, simple: 1`},
		{"array in open", `items: [1, 2, 3], count: 3`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parsers.ParseString(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse %q: %v", tt.name, err)
			}
			if doc == nil {
				t.Fatal("Expected non-nil document")
			}
		})
	}
}

// TestParseString_ErrorRecovery tests that errors are properly reported
func TestParseString_ErrorRecovery(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorCode   parsers.ErrorCode
	}{
		{"unclosed object", `{a: 1, b: 2`, true, parsers.ErrorExpectingBracket},
		{"unclosed array", `[1, 2, 3`, true, parsers.ErrorExpectingBracket},
		{"missing comma object", `{a: 1 b: 2}`, true, parsers.ErrorUnexpectedToken},
		{"duplicate section", "--- test\n{a: 1}\n--- test\n{b: 2}", true, parsers.ErrorDuplicateSection},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsers.ParseString(tt.input)

			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for %q but got none", tt.name)
					return
				}

				syntaxErr, ok := err.(*parsers.SyntaxError)
				if !ok {
					t.Errorf("Expected SyntaxError, got %T", err)
					return
				}

				if tt.errorCode != "" && syntaxErr.Code != tt.errorCode {
					t.Errorf("Expected error code %q, got %q", tt.errorCode, syntaxErr.Code)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %q: %v", tt.name, err)
				}
			}
		})
	}
}

// TestParseString_ConcurrentParsing tests thread safety
func TestParseString_ConcurrentParsing(t *testing.T) {
	input := `{name: "test", value: 42, items: [1, 2, 3]}`

	// Parse the same input concurrently
	const goroutines = 100
	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_, err := parsers.ParseString(input)
			errors <- err
		}()
	}

	// Check all goroutines completed without error
	for i := 0; i < goroutines; i++ {
		if err := <-errors; err != nil {
			t.Errorf("Concurrent parsing error: %v", err)
		}
	}
}

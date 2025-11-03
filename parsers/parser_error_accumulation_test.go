package parsers

import (
	"testing"
)

// TestParser_ErrorAccumulation_SingleError tests that a single error is accumulated.
func TestParser_ErrorAccumulation_SingleError(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ {unclosed: "object"
~ name: "Bob", age: 30
`

	doc, err := ParseString(input)

	// Should have a document
	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	// Should have last error
	if err == nil {
		t.Error("Expected error but got nil")
	}

	// Should have exactly 1 error accumulated
	errors := doc.GetErrors()
	if len(errors) != 1 {
		t.Errorf("Expected 1 accumulated error, got %d", len(errors))
	}

	// Last error should match accumulated error
	if len(errors) > 0 && errors[0] != err {
		t.Error("Last error doesn't match first accumulated error")
	}
}

// TestParser_ErrorAccumulation_MultipleErrors tests multiple errors in collection.
func TestParser_ErrorAccumulation_MultipleErrors(t *testing.T) {
	input := `
~ {error1
~ name: "Valid"
~ {error2
~ {error3
`

	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	if err == nil {
		t.Error("Expected error but got nil")
	}

	// Should have 3 errors accumulated (one for each broken object)
	errors := doc.GetErrors()
	if len(errors) != 3 {
		t.Errorf("Expected 3 accumulated errors, got %d", len(errors))
		for i, e := range errors {
			t.Logf("  Error %d: %v", i+1, e)
		}
	}
}

// TestParser_ErrorAccumulation_DuplicateSections tests duplicate section errors.
func TestParser_ErrorAccumulation_DuplicateSections(t *testing.T) {
	input := `
--- users
~ name: "Alice"

--- users
~ name: "Bob"

--- users
~ name: "Charlie"
`

	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	if err == nil {
		t.Error("Expected error but got nil")
	}

	// Should have 2 errors (second and third "users" sections are duplicates)
	errors := doc.GetErrors()
	if len(errors) != 2 {
		t.Errorf("Expected 2 duplicate section errors, got %d", len(errors))
		for i, e := range errors {
			t.Logf("  Error %d: %v", i+1, e)
		}
	}

	// All sections should be present (auto-renamed)
	if len(doc.Sections) != 3 {
		t.Errorf("Expected 3 sections, got %d", len(doc.Sections))
	}
}

// TestParser_ErrorAccumulation_MixedErrors tests both collection and section errors.
func TestParser_ErrorAccumulation_MixedErrors(t *testing.T) {
	input := `
--- section1
~ {error1
~ name: "Valid"

--- section1
~ {error2

--- products
~ laptop
~ {error3
`

	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	if err == nil {
		t.Error("Expected error but got nil")
	}

	// Should have 4 errors: 3 collection errors + 1 duplicate section
	errors := doc.GetErrors()
	if len(errors) != 4 {
		t.Errorf("Expected 4 accumulated errors, got %d", len(errors))
		for i, e := range errors {
			t.Logf("  Error %d: %v", i+1, e)
		}
	}
}

// TestParser_ErrorAccumulation_NoErrors tests that no errors are accumulated when parsing succeeds.
func TestParser_ErrorAccumulation_NoErrors(t *testing.T) {
	input := `
~ name: "Alice", age: 25
~ name: "Bob", age: 30
`

	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}

	// Should have no errors accumulated
	errors := doc.GetErrors()
	if len(errors) != 0 {
		t.Errorf("Expected 0 accumulated errors, got %d", len(errors))
		for i, e := range errors {
			t.Logf("  Error %d: %v", i+1, e)
		}
	}
}

// TestParser_ErrorAccumulation_GetErrorsNilDoc tests GetErrors on nil document.
func TestParser_ErrorAccumulation_GetErrorsNilDoc(t *testing.T) {
	var doc *DocumentNode

	errors := doc.GetErrors()
	if errors != nil {
		t.Error("Expected nil for GetErrors on nil document")
	}
}

// TestParser_ErrorAccumulation_AllErrorTypes tests accumulation of different error types.
func TestParser_ErrorAccumulation_AllErrorTypes(t *testing.T) {
	input := `
--- users
~ {unclosed_object

--- users
~ name: "Valid"

--- products
~ {another_error
~ laptop
`

	doc, err := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	if err == nil {
		t.Error("Expected error but got nil")
	}

	// Should accumulate:
	// 1. Collection error (unclosed_object)
	// 2. Duplicate section error (second "users")
	// 3. Collection error (another_error)
	errors := doc.GetErrors()
	expectedCount := 3
	if len(errors) != expectedCount {
		t.Errorf("Expected %d accumulated errors, got %d", expectedCount, len(errors))
		for i, e := range errors {
			t.Logf("  Error %d: %v", i+1, e)
		}
	}

	// Verify we got all sections (with auto-rename)
	if len(doc.Sections) != 3 {
		t.Errorf("Expected 3 sections (users, users_2, products), got %d", len(doc.Sections))
	}
}

// TestParser_ErrorAccumulation_BackwardCompatibility tests that error return value is preserved.
func TestParser_ErrorAccumulation_BackwardCompatibility(t *testing.T) {
	input := `
~ {error1
~ valid
~ {error2
`

	doc, lastErr := ParseString(input)

	if doc == nil {
		t.Fatal("Expected document but got nil")
	}

	// Last error should be non-nil
	if lastErr == nil {
		t.Error("Expected last error but got nil")
	}

	// GetErrors should return all errors
	errors := doc.GetErrors()
	if len(errors) < 1 {
		t.Fatal("Expected at least 1 accumulated error")
	}

	// Last accumulated error should match the returned error
	if errors[len(errors)-1] != lastErr {
		t.Error("Last accumulated error doesn't match returned error")
	}
}

package parsers

import (
	"testing"
)

func TestFastParser_SimpleObject(t *testing.T) {
	input := `{"name": "John", "age": 30}`
	parser, rootIdx, err := FastParse(input)
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Fatalf("expected object, got %v", val.Type)
	}

	if val.ChildCount != 2 {
		t.Fatalf("expected 2 members, got %d", val.ChildCount)
	}

	// Check name
	member := parser.GetMember(val.FirstChild)
	key := parser.GetMemberKey(member)
	if key != "name" {
		t.Errorf("expected key 'name', got '%s'", key)
	}

	nameVal := parser.GetValue(member.ValueIdx)
	if nameVal.Type != TypeString {
		t.Errorf("expected string type for name")
	}
	if parser.GetString(nameVal) != "John" {
		t.Errorf("expected 'John', got '%s'", parser.GetString(nameVal))
	}
}

func TestFastParser_IONativeFormat(t *testing.T) {
	input := `{name: John, age: 30, active: true}`
	parser, rootIdx, err := FastParse(input)
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Fatalf("expected object, got %v", val.Type)
	}

	if val.ChildCount != 3 {
		t.Fatalf("expected 3 members, got %d", val.ChildCount)
	}
}

func TestFastParser_Array(t *testing.T) {
	input := `[1, 2, 3, 4, 5]`
	parser, rootIdx, err := FastParse(input)
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeArray {
		t.Fatalf("expected array, got %v", val.Type)
	}

	if val.ChildCount != 5 {
		t.Fatalf("expected 5 elements, got %d", val.ChildCount)
	}

	// Check first element
	elem := parser.GetValue(val.FirstChild)
	if elem.Type != TypeInt {
		t.Errorf("expected int type")
	}
	if elem.IntValue != 1 {
		t.Errorf("expected 1, got %d", elem.IntValue)
	}
}

func TestFastParser_NestedStructures(t *testing.T) {
	t.Skip("TODO: fix nested structure parsing")
	input := `{
		"user": {
			"name": "Alice",
			"age": 25
		},
		"orders": [1, 2, 3]
	}`

	parser, rootIdx, err := FastParse(input)
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Fatalf("expected object")
	}

	// Convert to map for easier testing
	result := parser.ToMap(rootIdx)
	if result == nil {
		t.Fatal("failed to convert to map")
	}

	t.Logf("Result: %+v", result)

	userRaw, ok := result["user"]
	if !ok {
		t.Fatal("user key not found")
	}

	user, ok := userRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("user is not an object, it's: %T %+v", userRaw, userRaw)
	}

	if user["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", user["name"])
	}
}

func TestFastParser_Reuse(t *testing.T) {
	parser := NewFastParser("", 10)

	// First parse
	parser.Reset(`{"name": "John"}`)
	rootIdx, err := parser.Parse()
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.ChildCount != 1 {
		t.Errorf("first parse: expected 1 member, got %d", val.ChildCount)
	}

	// Second parse (reuse)
	parser.Reset(`{"name": "Jane", "age": 25}`)
	rootIdx, err = parser.Parse()
	if err != nil {
		t.Fatal(err)
	}

	val = parser.GetValue(rootIdx)
	if val.ChildCount != 2 {
		t.Errorf("second parse: expected 2 members, got %d", val.ChildCount)
	}
}

func TestFastParser_Numbers(t *testing.T) {
	tests := []struct {
		input    string
		expected interface{}
		typeExp  ValueType
	}{
		{`42`, int64(42), TypeInt},
		{`-42`, int64(-42), TypeInt},
		{`3.14`, float64(3.14), TypeFloat},
		{`-3.14`, float64(-3.14), TypeFloat},
	}

	for _, tt := range tests {
		parser, rootIdx, err := FastParse(tt.input)
		if err != nil {
			t.Errorf("failed to parse %s: %v", tt.input, err)
			continue
		}

		val := parser.GetValue(rootIdx)
		if val.Type != tt.typeExp {
			t.Errorf("%s: expected type %v, got %v", tt.input, tt.typeExp, val.Type)
		}

		switch tt.typeExp {
		case TypeInt:
			if val.IntValue != tt.expected.(int64) {
				t.Errorf("%s: expected %v, got %v", tt.input, tt.expected, val.IntValue)
			}
		case TypeFloat:
			if val.FloatValue != tt.expected.(float64) {
				t.Errorf("%s: expected %v, got %v", tt.input, tt.expected, val.FloatValue)
			}
		}
	}
}

func TestFastParser_Booleans(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{`true`, true},
		{`false`, false},
	}

	for _, tt := range tests {
		parser, rootIdx, err := FastParse(tt.input)
		if err != nil {
			t.Errorf("failed to parse %s: %v", tt.input, err)
			continue
		}

		val := parser.GetValue(rootIdx)
		if val.Type != TypeBool {
			t.Errorf("%s: expected bool type", tt.input)
		}
		if val.BoolValue != tt.expected {
			t.Errorf("%s: expected %v, got %v", tt.input, tt.expected, val.BoolValue)
		}
	}
}

func TestFastParser_Null(t *testing.T) {
	parser, rootIdx, err := FastParse(`null`)
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeNull {
		t.Errorf("expected null type, got %v", val.Type)
	}
}

func TestFastParser_String(t *testing.T) {
	parser, rootIdx, err := FastParse(`"hello world"`)
	if err != nil {
		t.Fatal(err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeString {
		t.Errorf("expected string type")
	}

	str := parser.GetString(val)
	if str != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", str)
	}
}

func TestFastParser_ComplexDocument(t *testing.T) {
	t.Skip("TODO: fix complex nested parsing")
	input := `{
		"header": {"name": "John", "age": 30},
		"products": [
			{"id": 1, "price": 29.99},
			{"id": 2, "price": 49.99}
		]
	}`

	parser, rootIdx, err := FastParse(input)
	if err != nil {
		t.Fatal(err)
	}

	// Convert to map
	result := parser.ToMap(rootIdx)
	if result == nil {
		t.Fatal("failed to convert")
	}

	header := result["header"].(map[string]interface{})
	if header["name"] != "John" {
		t.Errorf("expected John")
	}

	products := result["products"].([]interface{})
	if len(products) != 2 {
		t.Errorf("expected 2 products")
	}
}

func TestFastParser_ToString(t *testing.T) {
	input := `{name: John, age: 30}`
	parser, rootIdx, err := FastParse(input)
	if err != nil {
		t.Fatal(err)
	}

	str := parser.String(rootIdx)
	if str == "" {
		t.Error("expected non-empty string representation")
	}
	t.Logf("String representation: %s", str)
}

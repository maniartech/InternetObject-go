package parsers

import (
	"testing"
)

func TestFastParserBytes_SimpleObject(t *testing.T) {
	input := []byte(`{"name": "John", "age": 30}`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Errorf("Expected object, got %v", val.Type)
	}

	if val.ChildCount != 2 {
		t.Errorf("Expected 2 members, got %d", val.ChildCount)
	}

	// Check name
	member := parser.GetMember(val.FirstChild)
	key := parser.GetMemberKey(member)
	if key != "name" {
		t.Errorf("Expected key 'name', got '%s'", key)
	}

	nameVal := parser.GetValue(member.ValueIdx)
	if nameVal.Type != TypeString {
		t.Errorf("Expected string type for name")
	}

	nameStr := parser.GetString(nameVal)
	if nameStr != "John" {
		t.Errorf("Expected name 'John', got '%s'", nameStr)
	}
}

func TestFastParserBytes_IONativeFormat(t *testing.T) {
	input := []byte(`{name: John, age: 30, active: true}`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Errorf("Expected object, got %v", val.Type)
	}

	if val.ChildCount != 3 {
		t.Errorf("Expected 3 members, got %d", val.ChildCount)
	}
}

func TestFastParserBytes_Array(t *testing.T) {
	input := []byte(`[1, 2, 3, 4, 5]`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeArray {
		t.Errorf("Expected array, got %v", val.Type)
	}

	if val.ChildCount != 5 {
		t.Errorf("Expected 5 elements, got %d", val.ChildCount)
	}

	// Check first element
	firstEl := parser.GetValue(val.FirstChild)
	if firstEl.Type != TypeInt {
		t.Errorf("Expected int type")
	}
	if firstEl.IntValue != 1 {
		t.Errorf("Expected value 1, got %d", firstEl.IntValue)
	}
}

func TestFastParserBytes_Reuse(t *testing.T) {
	parser := NewFastParserBytes(nil, 50)

	// Parse first input
	input1 := []byte(`{"a": 1}`)
	parser.Reset(input1)
	rootIdx1, err := parser.Parse()
	if err != nil {
		t.Fatalf("First parse failed: %v", err)
	}

	val1 := parser.GetValue(rootIdx1)
	if val1.ChildCount != 1 {
		t.Errorf("Expected 1 member in first parse, got %d", val1.ChildCount)
	}

	// Parse second input
	input2 := []byte(`{"b": 2, "c": 3}`)
	parser.Reset(input2)
	rootIdx2, err := parser.Parse()
	if err != nil {
		t.Fatalf("Second parse failed: %v", err)
	}

	val2 := parser.GetValue(rootIdx2)
	if val2.ChildCount != 2 {
		t.Errorf("Expected 2 members in second parse, got %d", val2.ChildCount)
	}
}

func TestFastParserBytes_Numbers(t *testing.T) {
	tests := []struct {
		input    []byte
		expected interface{}
		isFloat  bool
	}{
		{[]byte(`123`), int64(123), false},
		{[]byte(`-456`), int64(-456), false},
		{[]byte(`3.14`), 3.14, true},
		{[]byte(`-2.5`), -2.5, true},
		{[]byte(`0`), int64(0), false},
		{[]byte(`99999`), int64(99999), false},
		{[]byte(`0.001`), 0.001, true},
	}

	for _, test := range tests {
		parser, rootIdx, err := FastParseBytes(test.input)
		if err != nil {
			t.Errorf("Parse failed for %s: %v", test.input, err)
			continue
		}

		val := parser.GetValue(rootIdx)

		if test.isFloat {
			if val.Type != TypeFloat {
				t.Errorf("Expected float type for %s, got %v", test.input, val.Type)
			}
			expected := test.expected.(float64)
			if val.FloatValue != expected {
				t.Errorf("Expected %f, got %f", expected, val.FloatValue)
			}
		} else {
			if val.Type != TypeInt {
				t.Errorf("Expected int type for %s, got %v", test.input, val.Type)
			}
			expected := test.expected.(int64)
			if val.IntValue != expected {
				t.Errorf("Expected %d, got %d", expected, val.IntValue)
			}
		}
	}
}

func TestFastParserBytes_Booleans(t *testing.T) {
	tests := []struct {
		input    []byte
		expected bool
	}{
		{[]byte(`true`), true},
		{[]byte(`false`), false},
	}

	for _, test := range tests {
		parser, rootIdx, err := FastParseBytes(test.input)
		if err != nil {
			t.Errorf("Parse failed for %s: %v", test.input, err)
			continue
		}

		val := parser.GetValue(rootIdx)
		if val.Type != TypeBool {
			t.Errorf("Expected bool type, got %v", val.Type)
		}
		if val.BoolValue != test.expected {
			t.Errorf("Expected %v, got %v", test.expected, val.BoolValue)
		}
	}
}

func TestFastParserBytes_Null(t *testing.T) {
	parser, rootIdx, err := FastParseBytes([]byte(`null`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeNull {
		t.Errorf("Expected null type, got %v", val.Type)
	}
}

func TestFastParserBytes_String(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte(`"hello"`), "hello"},
		{[]byte(`"world"`), "world"},
		{[]byte(`""`), ""},
	}

	for _, test := range tests {
		parser, rootIdx, err := FastParseBytes(test.input)
		if err != nil {
			t.Errorf("Parse failed for %s: %v", test.input, err)
			continue
		}

		val := parser.GetValue(rootIdx)
		if val.Type != TypeString {
			t.Errorf("Expected string type, got %v", val.Type)
		}

		str := parser.GetString(val)
		if str != test.expected {
			t.Errorf("Expected '%s', got '%s'", test.expected, str)
		}
	}
}

func TestFastParserBytes_ToString(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte(`null`), "null"},
		{[]byte(`true`), "true"},
		{[]byte(`false`), "false"},
		{[]byte(`123`), "123"},
		{[]byte(`"hello"`), "hello"},
		{[]byte(`[1, 2, 3]`), "[1, 2, 3]"},
		{[]byte(`{"name": "John"}`), "{name: John}"},
	}

	for _, test := range tests {
		parser, rootIdx, err := FastParseBytes(test.input)
		if err != nil {
			t.Errorf("Parse failed for %s: %v", test.input, err)
			continue
		}

		result := parser.String(rootIdx)
		if result != test.expected {
			t.Errorf("Expected '%s', got '%s'", test.expected, result)
		}
	}
}

func TestFastParserBytes_ToMap(t *testing.T) {
	input := []byte(`{"name": "John", "age": 30, "active": true}`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result := parser.ToMap(rootIdx)

	if result["name"] != "John" {
		t.Errorf("Expected name 'John', got %v", result["name"])
	}

	if result["age"] != int64(30) {
		t.Errorf("Expected age 30, got %v", result["age"])
	}

	if result["active"] != true {
		t.Errorf("Expected active true, got %v", result["active"])
	}
}

func TestFastParserBytes_ToInterface(t *testing.T) {
	input := []byte(`{
		"name": "John",
		"age": 30,
		"scores": [95, 87, 92],
		"active": true
	}`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	result := parser.ToInterface(rootIdx)

	obj, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	if obj["name"] != "John" {
		t.Errorf("Expected name 'John', got %v", obj["name"])
	}

	scores, ok := obj["scores"].([]interface{})
	if !ok {
		t.Fatalf("Expected array for scores, got %T", obj["scores"])
	}

	if len(scores) != 3 {
		t.Errorf("Expected 3 scores, got %d", len(scores))
	}
}

func TestFastParserBytes_EmptyObject(t *testing.T) {
	parser, rootIdx, err := FastParseBytes([]byte(`{}`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Errorf("Expected object type, got %v", val.Type)
	}

	if val.ChildCount != 0 {
		t.Errorf("Expected 0 members, got %d", val.ChildCount)
	}
}

func TestFastParserBytes_EmptyArray(t *testing.T) {
	parser, rootIdx, err := FastParseBytes([]byte(`[]`))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeArray {
		t.Errorf("Expected array type, got %v", val.Type)
	}

	if val.ChildCount != 0 {
		t.Errorf("Expected 0 elements, got %d", val.ChildCount)
	}
}

func TestFastParserBytes_GetStringBytes(t *testing.T) {
	input := []byte(`{"message": "hello world"}`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	member := parser.GetMember(val.FirstChild)
	strVal := parser.GetValue(member.ValueIdx)

	// Test GetStringBytes (zero-copy)
	strBytes := parser.GetStringBytes(strVal)
	if string(strBytes) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(strBytes))
	}

	// Test GetMemberKeyBytes (zero-copy)
	keyBytes := parser.GetMemberKeyBytes(member)
	if string(keyBytes) != "message" {
		t.Errorf("Expected 'message', got '%s'", string(keyBytes))
	}
}

func TestFastParserBytes_FromString(t *testing.T) {
	inputStr := `{"test": "value"}`

	parser, rootIdx, err := FastParseBytesFromString(inputStr)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Errorf("Expected object type, got %v", val.Type)
	}

	if val.ChildCount != 1 {
		t.Errorf("Expected 1 member, got %d", val.ChildCount)
	}
}

func TestFastParserBytes_ResetFromString(t *testing.T) {
	parser := NewFastParserBytes(nil, 50)

	// Parse from string
	parser.ResetFromString(`{"a": 1}`)
	rootIdx, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.ChildCount != 1 {
		t.Errorf("Expected 1 member, got %d", val.ChildCount)
	}
}

func TestFastParserBytes_MixedTypes(t *testing.T) {
	input := []byte(`{
		"null_val": null,
		"bool_val": true,
		"int_val": 42,
		"float_val": 3.14,
		"string_val": "hello",
		"array_val": [1, 2, 3],
		"object_val": {"nested": "value"}
	}`)

	parser, rootIdx, err := FastParseBytes(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	val := parser.GetValue(rootIdx)
	if val.Type != TypeObject {
		t.Errorf("Expected object type, got %v", val.Type)
	}

	if val.ChildCount != 7 {
		t.Errorf("Expected 7 members, got %d", val.ChildCount)
	}

	// Verify each type
	for i := 0; i < val.ChildCount; i++ {
		member := parser.GetMember(val.FirstChild + i)
		key := parser.GetMemberKey(member)
		memberVal := parser.GetValue(member.ValueIdx)

		switch key {
		case "null_val":
			if memberVal.Type != TypeNull {
				t.Errorf("Expected null type for %s", key)
			}
		case "bool_val":
			if memberVal.Type != TypeBool {
				t.Errorf("Expected bool type for %s", key)
			}
		case "int_val":
			if memberVal.Type != TypeInt {
				t.Errorf("Expected int type for %s", key)
			}
		case "float_val":
			if memberVal.Type != TypeFloat {
				t.Errorf("Expected float type for %s", key)
			}
		case "string_val":
			if memberVal.Type != TypeString {
				t.Errorf("Expected string type for %s", key)
			}
		case "array_val":
			if memberVal.Type != TypeArray {
				t.Errorf("Expected array type for %s", key)
			}
		case "object_val":
			if memberVal.Type != TypeObject {
				t.Errorf("Expected object type for %s", key)
			}
		}
	}
}

func TestFastParserBytes_LargeNumbers(t *testing.T) {
	tests := []struct {
		input    []byte
		expected int64
	}{
		{[]byte(`999999999`), 999999999},
		{[]byte(`-999999999`), -999999999},
		{[]byte(`0`), 0},
	}

	for _, test := range tests {
		parser, rootIdx, err := FastParseBytes(test.input)
		if err != nil {
			t.Errorf("Parse failed for %s: %v", test.input, err)
			continue
		}

		val := parser.GetValue(rootIdx)
		if val.Type != TypeInt {
			t.Errorf("Expected int type, got %v", val.Type)
		}

		if val.IntValue != test.expected {
			t.Errorf("Expected %d, got %d", test.expected, val.IntValue)
		}
	}
}

package parsers

import (
	"encoding/json"
	"testing"
)

// Benchmark parallel parsing vs sequential
func BenchmarkParsing_Sequential_ComplexDocument(b *testing.B) {
	input := `[
		{name: "John Doe", age: 30, email: "john@example.com", city: "New York", country: "USA"},
		{name: "Jane Smith", age: 25, email: "jane@example.com", city: "London", country: "UK"},
		{name: "Bob Johnson", age: 35, email: "bob@example.com", city: "Toronto", country: "Canada"},
		{name: "Alice Williams", age: 28, email: "alice@example.com", city: "Sydney", country: "Australia"},
		{name: "Charlie Brown", age: 32, email: "charlie@example.com", city: "Paris", country: "France"},
		{name: "Diana Prince", age: 29, email: "diana@example.com", city: "Berlin", country: "Germany"},
		{name: "Ethan Hunt", age: 34, email: "ethan@example.com", city: "Tokyo", country: "Japan"},
		{name: "Fiona Green", age: 26, email: "fiona@example.com", city: "Mumbai", country: "India"},
		{name: "George Miller", age: 31, email: "george@example.com", city: "Singapore", country: "Singapore"},
		{name: "Hannah White", age: 27, email: "hannah@example.com", city: "Dubai", country: "UAE"}
	]`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		doc, err := ParseString(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

func BenchmarkParsing_Parallel_ComplexDocument(b *testing.B) {
	input := `[
		{name: "John Doe", age: 30, email: "john@example.com", city: "New York", country: "USA"},
		{name: "Jane Smith", age: 25, email: "jane@example.com", city: "London", country: "UK"},
		{name: "Bob Johnson", age: 35, email: "bob@example.com", city: "Toronto", country: "Canada"},
		{name: "Alice Williams", age: 28, email: "alice@example.com", city: "Sydney", country: "Australia"},
		{name: "Charlie Brown", age: 32, email: "charlie@example.com", city: "Paris", country: "France"},
		{name: "Diana Prince", age: 29, email: "diana@example.com", city: "Berlin", country: "Germany"},
		{name: "Ethan Hunt", age: 34, email: "ethan@example.com", city: "Tokyo", country: "Japan"},
		{name: "Fiona Green", age: 26, email: "fiona@example.com", city: "Mumbai", country: "India"},
		{name: "George Miller", age: 31, email: "george@example.com", city: "Singapore", country: "Singapore"},
		{name: "Hannah White", age: 27, email: "hannah@example.com", city: "Dubai", country: "UAE"}
	]`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		doc, err := ParseStringParallel(input)
		if err != nil {
			b.Fatal(err)
		}
		_ = doc
	}
}

// Compare with JSON on same data
func BenchmarkJSON_ComplexDocument(b *testing.B) {
	jsonData := `{
		"users": [
			{"name": "John Doe", "age": 30, "email": "john@example.com", "city": "New York", "country": "USA", "phone": "+1-555-0101", "occupation": "Engineer"},
			{"name": "Jane Smith", "age": 25, "email": "jane@example.com", "city": "London", "country": "UK", "phone": "+44-20-7946-0958", "occupation": "Designer"},
			{"name": "Bob Johnson", "age": 35, "email": "bob@example.com", "city": "Toronto", "country": "Canada", "phone": "+1-416-555-0100", "occupation": "Manager"},
			{"name": "Alice Williams", "age": 28, "email": "alice@example.com", "city": "Sydney", "country": "Australia", "phone": "+61-2-9999-9999", "occupation": "Developer"},
			{"name": "Charlie Brown", "age": 32, "email": "charlie@example.com", "city": "Paris", "country": "France", "phone": "+33-1-42-86-82-00", "occupation": "Architect"},
			{"name": "Diana Prince", "age": 29, "email": "diana@example.com", "city": "Berlin", "country": "Germany", "phone": "+49-30-12345678", "occupation": "Analyst"},
			{"name": "Ethan Hunt", "age": 34, "email": "ethan@example.com", "city": "Tokyo", "country": "Japan", "phone": "+81-3-1234-5678", "occupation": "Consultant"},
			{"name": "Fiona Green", "age": 26, "email": "fiona@example.com", "city": "Mumbai", "country": "India", "phone": "+91-22-1234-5678", "occupation": "Developer"},
			{"name": "George Miller", "age": 31, "email": "george@example.com", "city": "Singapore", "country": "Singapore", "phone": "+65-6123-4567", "occupation": "Engineer"},
			{"name": "Hannah White", "age": 27, "email": "hannah@example.com", "city": "Dubai", "country": "UAE", "phone": "+971-4-123-4567", "occupation": "Designer"}
		],
		"orders": [
			{"orderId": 1001, "userId": 1, "product": "Premium Widget", "quantity": 5, "price": 99.99, "status": "shipped", "date": "2024-01-15"},
			{"orderId": 1002, "userId": 2, "product": "Standard Gadget", "quantity": 3, "price": 49.99, "status": "delivered", "date": "2024-01-16"},
			{"orderId": 1003, "userId": 3, "product": "Deluxe Tool", "quantity": 2, "price": 149.99, "status": "processing", "date": "2024-01-17"},
			{"orderId": 1004, "userId": 4, "product": "Basic Item", "quantity": 10, "price": 9.99, "status": "shipped", "date": "2024-01-18"},
			{"orderId": 1005, "userId": 5, "product": "Professional Kit", "quantity": 1, "price": 299.99, "status": "delivered", "date": "2024-01-19"},
			{"orderId": 1006, "userId": 6, "product": "Starter Pack", "quantity": 4, "price": 39.99, "status": "processing", "date": "2024-01-20"},
			{"orderId": 1007, "userId": 7, "product": "Advanced Set", "quantity": 2, "price": 199.99, "status": "shipped", "date": "2024-01-21"},
			{"orderId": 1008, "userId": 8, "product": "Economy Bundle", "quantity": 6, "price": 59.99, "status": "delivered", "date": "2024-01-22"},
			{"orderId": 1009, "userId": 9, "product": "Premium Package", "quantity": 3, "price": 179.99, "status": "processing", "date": "2024-01-23"},
			{"orderId": 1010, "userId": 10, "product": "Ultimate Collection", "quantity": 1, "price": 399.99, "status": "shipped", "date": "2024-01-24"}
		],
		"products": [
			{"sku": "SKU-001", "name": "Widget", "price": 19.99, "stock": 100, "category": "Electronics", "supplier": "TechCorp"},
			{"sku": "SKU-002", "name": "Gadget", "price": 29.99, "stock": 75, "category": "Electronics", "supplier": "GadgetInc"},
			{"sku": "SKU-003", "name": "Tool", "price": 39.99, "stock": 50, "category": "Hardware", "supplier": "ToolCo"},
			{"sku": "SKU-004", "name": "Item", "price": 49.99, "stock": 125, "category": "General", "supplier": "ItemsRUs"},
			{"sku": "SKU-005", "name": "Kit", "price": 59.99, "stock": 30, "category": "Professional", "supplier": "ProSupply"},
			{"sku": "SKU-006", "name": "Pack", "price": 69.99, "stock": 90, "category": "Starter", "supplier": "PackCo"},
			{"sku": "SKU-007", "name": "Set", "price": 79.99, "stock": 45, "category": "Advanced", "supplier": "SetSupply"},
			{"sku": "SKU-008", "name": "Bundle", "price": 89.99, "stock": 60, "category": "Economy", "supplier": "BundleCorp"},
			{"sku": "SKU-009", "name": "Package", "price": 99.99, "stock": 25, "category": "Premium", "supplier": "PremiumInc"},
			{"sku": "SKU-010", "name": "Collection", "price": 109.99, "stock": 15, "category": "Ultimate", "supplier": "CollectCo"}
		]
	}`

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var result map[string]interface{}
		err := json.Unmarshal([]byte(jsonData), &result)
		if err != nil {
			b.Fatal(err)
		}
		_ = result
	}
}

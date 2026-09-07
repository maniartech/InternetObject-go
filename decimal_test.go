package internetobject_test

import (
	"math/big"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The type a caller actually reaches for. These tests use only the public
// surface, so they fail if an operation is not exported or an alias breaks.
func TestDecimalIsUsableFromThePublicPackage(t *testing.T) {
	price, err := io.ParseDecimal("19.99")
	if err != nil {
		t.Fatal(err)
	}
	qty := io.NewDecimal(3, 0)

	total := price.Mul(qty)
	if total.String() != "59.97" {
		t.Errorf("19.99 * 3 = %s", total.String())
	}
	// A tax rate that would vanish in the reference's multiply.
	rate, _ := io.ParseDecimal("0.08")
	tax := total.Mul(rate).Round(2)
	if tax.String() != "4.80" {
		t.Errorf("tax = %s, want 4.80", tax.String())
	}
	if got := total.Add(tax).String(); got != "64.77" {
		t.Errorf("total with tax = %s", got)
	}
	if _, err := price.Quo(io.NewDecimal(0, 0), 2); err != io.ErrDivideByZero {
		t.Errorf("divide by zero: %v", err)
	}
	if got := io.DecimalFromBig(big.NewInt(1999), 2); !got.Same(price) {
		t.Errorf("DecimalFromBig = %s", got.String())
	}
}

// A decimal read from a document is the same type, with the same operations —
// the point of putting the behaviour on the value rather than in the validator.
func TestDecimalFromAParsedDocument(t *testing.T) {
	// The `--- $S` selector is what BINDS the named schema; a bare `---`
	// would leave the section schema-less and its members positional.
	doc, err := io.Parse("~ $S: {price: decimal, qty: int}\n--- $S\n~ 19.99m, 3")
	if err != nil {
		t.Fatal(err)
	}
	rec := doc.Records()[0].(*io.Object)
	v, ok := rec.Get("price")
	if !ok {
		t.Fatal("no price member")
	}
	price, ok := v.(io.Decimal)
	if !ok {
		t.Fatalf("price is %T, want io.Decimal", v)
	}
	if price.String() != "19.99" || price.Scale != 2 || price.Precision() != 4 {
		t.Errorf("price = %s scale=%d precision=%d", price.String(), price.Scale, price.Precision())
	}
	if got := price.Mul(io.NewDecimal(3, 0)).String(); got != "59.97" {
		t.Errorf("parsed decimal does not multiply: %s", got)
	}
	// Scale survives the round trip through the writer.
	back, err := io.Parse(doc.String())
	if err != nil {
		t.Fatalf("%v for %q", err, doc.String())
	}
	again, _ := back.Records()[0].(*io.Object).Get("price")
	if !again.(io.Decimal).Same(price) {
		t.Errorf("scale did not survive the round trip: %v vs %v", again, price)
	}
	if doc.String() != back.String() {
		t.Errorf("writing is not idempotent: %q then %q", doc.String(), back.String())
	}
}

// Struct binding both directions, since Decimal is a model type the marshaler
// must not reflect over.
func TestDecimalBindsToStructs(t *testing.T) {
	type Line struct {
		Price io.Decimal `io:"price"`
		Qty   int        `io:"qty"`
	}
	var l Line
	if err := io.Unmarshal("price: decimal, qty: int\n---\n~ 1.50m, 2", &l); err != nil {
		t.Fatal(err)
	}
	if l.Price.String() != "1.50" {
		t.Errorf("bound price = %s", l.Price.String())
	}
	text, err := io.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	// The scale the value carries is the scale that gets written.
	if text != "price: decimal, qty: int\n---\n1.50m, 2" {
		t.Errorf("marshal = %q", text)
	}
	// And a value built in Go, never parsed, writes the same way.
	built := Line{Price: io.NewDecimal(150, 2), Qty: 2}
	if got, _ := io.Marshal(built); got != text {
		t.Errorf("built value marshals differently:\n got %q\nwant %q", got, text)
	}
}

// Validation now runs through the type's own operations, so the constraint
// codes must still be exactly what the corpus pins.
func TestDecimalConstraintsStillReportTheirCodes(t *testing.T) {
	for _, tc := range []struct {
		schema, value string
		code          io.Code
	}{
		{"{decimal, min: 10m}", "5m", "mismatched-min"},
		{"{decimal, max: 10m}", "50m", "mismatched-max"},
		{"{decimal, multipleOf: 5m}", "16m", "mismatched-multiple-of"},
		{"{decimal, scale: 2}", "1.5m", "mismatched-scale"},
		{"{decimal, precision: 4, scale: 2}", "12345.67m", "mismatched-precision"},
		{"{decimal, precision: 1}", "0.05m", "mismatched-precision"},
	} {
		_, err := io.Parse("d: " + tc.schema + "\n---\n~ " + tc.value)
		if err == nil {
			t.Errorf("%s with %s: expected %s, got no error", tc.schema, tc.value, tc.code)
			continue
		}
		var list io.ErrorList
		if !asErrorList(err, &list) {
			t.Fatalf("not an ErrorList: %T", err)
		}
		if !list.Has(tc.code) {
			t.Errorf("%s with %s: got %v, want %s", tc.schema, tc.value, list.Codes(), tc.code)
		}
	}
	// Bounds compare by MAGNITUDE, so a differently scaled bound still works.
	if _, err := io.Parse("d: {decimal, min: 10.00m}\n---\n~ 10m"); err != nil {
		t.Errorf("10m should satisfy min 10.00m: %v", err)
	}
}

func asErrorList(err error, out *io.ErrorList) bool {
	l, ok := err.(io.ErrorList)
	if ok {
		*out = l
	}
	return ok
}

// The Decimal operators are LOAD-BEARING, not conveniences: validation routes
// every constraint the decimal typedef declares through them. This test exists
// so none can be deleted as "unused public API" without the failure being
// obvious and named.
func TestValidationUsesTheDecimalOperators(t *testing.T) {
	for _, tc := range []struct {
		constraint, schema, value string
		code                      io.Code
		operator                  string
	}{
		{"min", "{decimal, min: 10m}", "5m", io.MismatchedMin, "Cmp"},
		{"max", "{decimal, max: 10m}", "50m", io.MismatchedMax, "Cmp"},
		{"multipleOf", "{decimal, multipleOf: 5m}", "16m", io.MismatchedMultipleOf, "IsMultipleOf"},
		{"precision", "{decimal, precision: 2}", "123m", io.MismatchedPrecision, "Precision"},
		{"scale", "{decimal, scale: 2}", "1.5m", io.MismatchedScale, "the Scale field"},
		{"choices", "{decimal, choices: [1.5m, 2m]}", "3m", io.MismatchedChoice, "Same"},
	} {
		_, err := io.Parse("d: " + tc.schema + "\n---\n~ " + tc.value)
		if err == nil {
			t.Errorf("%s (via %s): %s accepted %s", tc.constraint, tc.operator, tc.schema, tc.value)
			continue
		}
		var list io.ErrorList
		if !asErrorList(err, &list) || !list.Has(tc.code) {
			t.Errorf("%s (via %s): got %v, want %s", tc.constraint, tc.operator, err, tc.code)
		}
	}

	// And each one accepts what it should, so the test cannot pass by
	// rejecting everything.
	for _, ok := range []struct{ schema, value string }{
		{"{decimal, min: 10m}", "10m"},
		{"{decimal, max: 10m}", "10m"},
		{"{decimal, multipleOf: 5m}", "15m"},
		{"{decimal, precision: 3}", "123m"},
		{"{decimal, scale: 2}", "1.50m"},
		{"{decimal, choices: [1.5m, 2m]}", "1.5m"},
		// Bounds compare by MAGNITUDE, so a differently scaled bound works.
		{"{decimal, min: 10.00m}", "10m"},
		{"{decimal, multipleOf: 5.0m}", "15m"},
	} {
		if _, err := io.Parse("d: " + ok.schema + "\n---\n~ " + ok.value); err != nil {
			t.Errorf("%s rejected %s: %v", ok.schema, ok.value, err)
		}
	}
}

// A decimal must reach a caller's own types, and come back, without losing
// what makes it a decimal.
func TestDecimalBindsToAndFromNativeFields(t *testing.T) {
	const typed = "v: decimal\n---\n19.99m"

	var asDecimal struct {
		V io.Decimal `io:"v"`
	}
	if err := io.Unmarshal(typed, &asDecimal); err != nil || asDecimal.V.String() != "19.99" {
		t.Errorf("into Decimal: %v, %v", asDecimal.V, err)
	}
	var asString struct {
		V string `io:"v"`
	}
	if err := io.Unmarshal(typed, &asString); err != nil || asString.V != "19.99" {
		t.Errorf("into string: %q, %v", asString.V, err)
	}
	var asFloat struct {
		V float64 `io:"v"`
	}
	if err := io.Unmarshal(typed, &asFloat); err != nil || asFloat.V != 19.99 {
		t.Errorf("into float64: %v, %v", asFloat.V, err)
	}
	// An integer field takes a decimal only when it IS one.
	var asInt struct {
		V int64 `io:"v"`
	}
	if err := io.Unmarshal(typed, &asInt); err == nil {
		t.Error("19.99m was stored in an int64")
	}
	if err := io.Unmarshal("v: decimal\n---\n42.00m", &asInt); err != nil || asInt.V != 42 {
		t.Errorf("42.00m into int64: %v, %v", asInt.V, err)
	}

	// Coming the other way: a Decimal field accepts what a schema-less
	// document actually carries, converted through TEXT so nothing is lost.
	for _, tc := range []struct{ name, src, want string }{
		{"number", "---\nv: 19.99", "19.99"},
		{"string", "---\nv: \"19.99\"", "19.99"},
		{"bigint", "---\nv: 42n", "42"},
		{"integer", "---\nv: 42", "42"},
	} {
		var into struct {
			V io.Decimal `io:"v"`
		}
		if err := io.Unmarshal(tc.src, &into); err != nil || into.V.String() != tc.want {
			t.Errorf("%s -> Decimal: %v (%q), %v", tc.name, into.V, into.V.String(), err)
		}
	}
	var bad struct {
		V io.Decimal `io:"v"`
	}
	if err := io.Unmarshal("---\nv: notanumber", &bad); err == nil {
		t.Error("a non-numeric string was stored in a Decimal")
	}

	// And a Decimal field marshals back to a decimal literal, scale intact.
	text, err := io.Marshal(asDecimal)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(text, "19.99m") {
		t.Errorf("marshal lost the decimal: %q", text)
	}
}

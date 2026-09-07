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
	for _, tc := range []struct{ schema, value, code string }{
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

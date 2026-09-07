package internetobject_test

import (
	"bytes"
	"strings"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

type SEmp struct {
	Name string `io:"name"`
	Age  int    `io:"age"`
}

func empStreamSchema(t *testing.T) *io.Schema {
	t.Helper()
	s, err := io.ParseSchema("{name: string, age: {int, min: 0}}")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// The property that matters: whatever this writes, this package's own reader
// reads back. A port that can consume a stream but not produce one cannot sit
// on both ends of a link.
func TestStreamMarshalerRoundTripsThroughItsOwnReader(t *testing.T) {
	var buf bytes.Buffer
	sm, err := io.NewStreamMarshaler(&buf, &io.StreamOptions{Schema: empStreamSchema(t)})
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.Marshal(SEmp{"Alice", 30}); err != nil {
		t.Fatalf("struct record: %v", err)
	}
	if err := sm.Marshal(map[string]any{"name": "Bob", "age": 41}); err != nil {
		t.Fatalf("map record: %v", err)
	}
	if err := sm.Marshal(io.NewObject(2).Append("name", "Cara").Append("age", 27)); err != nil {
		t.Fatalf("Object record: %v", err)
	}
	if err := sm.Close(); err != nil {
		t.Fatal(err)
	}

	var names []string
	for item, err := range io.Stream(bytes.NewReader(buf.Bytes()), nil) {
		if err != nil {
			t.Fatalf("the reader could not read what the writer wrote: %v\n%s", err, buf.String())
		}
		if item.Err != nil {
			t.Fatalf("record %d faulted on read: %s\n%s", item.Index, item.Err.Code, buf.String())
		}
		v, ok := item.Value.(*io.Object).Get("name")
		if !ok {
			t.Fatalf("record %d lost its member names: %v", item.Index, item.Value)
		}
		names = append(names, v.(string))
	}
	if len(names) != 3 || names[0] != "Alice" || names[2] != "Cara" {
		t.Errorf("read back %v", names)
	}
	// It is also an ordinary document.
	if _, err := io.Parse(buf.String()); err != nil {
		t.Errorf("the stream is not a valid document: %v\n%s", err, buf.String())
	}
}

// A record that fails validation is not written at all, so the stream never
// carries a row its reader would reject.
func TestStreamMarshalerRejectsWithoutWriting(t *testing.T) {
	var buf bytes.Buffer
	sm, _ := io.NewStreamMarshaler(&buf, &io.StreamOptions{Schema: empStreamSchema(t)})
	if err := sm.Marshal(SEmp{"Alice", 30}); err != nil {
		t.Fatal(err)
	}
	err := sm.Marshal(SEmp{"Bad", -5})
	if err == nil {
		t.Fatal("a record violating the schema was written")
	}
	var list io.ErrorList
	if !asErrorList(err, &list) || !list.Has(io.MismatchedMin) {
		t.Errorf("got %v, want mismatched-min", err)
	}
	if err := sm.Marshal(SEmp{"Cara", 27}); err != nil {
		t.Fatalf("the writer did not recover from a rejected record: %v", err)
	}
	if err := sm.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "Bad") {
		t.Errorf("the rejected record reached the wire:\n%s", buf.String())
	}
	n := 0
	for item, err := range io.Stream(bytes.NewReader(buf.Bytes()), nil) {
		if err != nil || item.Err != nil {
			t.Fatalf("stream unreadable after a rejection: %v %v", err, item.Err)
		}
		n++
	}
	if n != 2 {
		t.Errorf("read %d records, want 2", n)
	}
}

// io-specs: the header is emitted at most once, before the first record, and
// the `---` terminator is always emitted — never the legacy header-less form.
func TestStreamMarshalerHeaderRules(t *testing.T) {
	var buf bytes.Buffer
	sm, _ := io.NewStreamMarshaler(&buf, &io.StreamOptions{Schema: empStreamSchema(t)})
	for i := 0; i < 3; i++ {
		if err := sm.Marshal(SEmp{"A", i}); err != nil {
			t.Fatal(err)
		}
	}
	sm.Close()
	out := buf.String()
	if n := strings.Count(out, "name: string"); n != 1 {
		t.Errorf("header written %d times:\n%s", n, out)
	}
	if n := strings.Count(out, "\n---\n"); n != 1 {
		t.Errorf("%d separators:\n%s", n, out)
	}
	if strings.Index(out, "~") < strings.Index(out, "---") {
		t.Errorf("a record was written before the terminator:\n%s", out)
	}
}

// A stream that carries no records is still a well-formed empty document.
func TestEmptyStreamIsWellFormed(t *testing.T) {
	var buf bytes.Buffer
	sm, _ := io.NewStreamMarshaler(&buf, nil)
	if err := sm.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "---\n" {
		t.Errorf("empty stream = %q, want %q", buf.String(), "---\n")
	}
	if _, err := io.Parse(buf.String()); err != nil {
		t.Errorf("the empty stream does not parse: %v", err)
	}
	// Close is idempotent and does not write twice.
	before := buf.Len()
	if err := sm.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if buf.Len() != before {
		t.Error("a second Close wrote more bytes")
	}
	if err := sm.Marshal(SEmp{"A", 1}); err == nil {
		t.Error("Marshal after Close was accepted")
	}
}

// A heterogeneous stream: several entity types, each under its own schema,
// with the switch emitted only where the shape actually changes.
func TestStreamMarshalerSchemaSwitching(t *testing.T) {
	defsText := "~ $Emp: {name: string, age: int}\n~ $Alert: {level: string, msg: string}"
	var buf bytes.Buffer
	sm, err := io.NewStreamMarshaler(&buf, &io.StreamOptions{Definitions: defsText})
	if err != nil {
		t.Fatal(err)
	}
	if err := sm.MarshalAs(map[string]any{"name": "Alice", "age": 30}, "Emp"); err != nil {
		t.Fatal(err)
	}
	if err := sm.MarshalAs(map[string]any{"name": "Bob", "age": 41}, "$Emp"); err != nil {
		t.Fatal(err)
	}
	if err := sm.MarshalAs(map[string]any{"level": "warn", "msg": "disk"}, "Alert"); err != nil {
		t.Fatal(err)
	}
	if err := sm.Close(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// Two consecutive Emp records share one switch.
	if n := strings.Count(out, "--- $Emp"); n != 1 {
		t.Errorf("$Emp switched %d times (should be once):\n%s", n, out)
	}
	if n := strings.Count(out, "--- $Alert"); n != 1 {
		t.Errorf("$Alert switched %d times:\n%s", n, out)
	}

	var got []string
	for item, err := range io.Stream(bytes.NewReader(buf.Bytes()), nil) {
		if err != nil {
			t.Fatalf("unreadable: %v\n%s", err, out)
		}
		if item.Err != nil {
			t.Fatalf("record %d: %s\n%s", item.Index, item.Err.Code, out)
		}
		got = append(got, item.SchemaName)
	}
	if len(got) != 3 || got[0] != "$Emp" || got[2] != "$Alert" {
		t.Errorf("schema names read back = %v", got)
	}

	// A name the stream's header does not define is refused.
	var b2 bytes.Buffer
	sm2, _ := io.NewStreamMarshaler(&b2, &io.StreamOptions{Definitions: defsText})
	if err := sm2.MarshalAs(map[string]any{"x": 1}, "NoSuch"); err == nil {
		t.Error("a switch to an undefined schema was accepted")
	}
	if err := sm2.MarshalAs(map[string]any{"x": 1}, ""); err == nil {
		t.Error("an empty schema name was accepted")
	}
}

// With no schema at all, records are written unvalidated and read back by key.
func TestStreamMarshalerWithoutASchema(t *testing.T) {
	var buf bytes.Buffer
	sm, _ := io.NewStreamMarshaler(&buf, nil)
	if err := sm.Marshal(map[string]any{"anything": "goes"}); err != nil {
		t.Fatal(err)
	}
	if err := sm.Close(); err != nil {
		t.Fatal(err)
	}
	n := 0
	for item, err := range io.Stream(bytes.NewReader(buf.Bytes()), nil) {
		if err != nil || item.Err != nil {
			t.Fatalf("%v %v (%q)", err, item.Err, buf.String())
		}
		if v, ok := item.Value.(*io.Object).Get("anything"); !ok || v != "goes" {
			t.Errorf("record = %v", item.Value)
		}
		n++
	}
	if n != 1 {
		t.Errorf("read %d records", n)
	}
}

func TestNewStreamMarshalerRejectsANilWriter(t *testing.T) {
	if _, err := io.NewStreamMarshaler(nil, nil); err == nil {
		t.Error("a nil writer was accepted")
	}
}

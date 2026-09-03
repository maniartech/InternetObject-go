package internetobject_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	io "github.com/maniartech/InternetObject-go"
)

// The compiled header is memoized on the header text (internal/document,
// headerFor). These gate the two things a memo can get wrong: handing one
// document's schema to another, and being written from two goroutines.
//
// `IO_NO_HEADER_CACHE=1 go test ./...` forces the uncached path, which is how
// the two are held identical — the same discipline as IO_NO_LAZY and
// IO_NO_FAST_PATH.

// Distinct headers must not collide, including headers that differ only in a
// type, only in a member name, or only in trailing whitespace.
func TestHeaderCacheDistinguishesHeaders(t *testing.T) {
	for _, tc := range []struct {
		src     string
		wantErr bool
	}{
		{"a: int\n---\n42", false},
		{"a: string\n---\n42", true}, // same shape, different type
		{"a: int\n---\n" + `"x"`, true},
		{"b: int\n---\n42", false},  // same type, different name
		{"a: int \n---\n42", false}, // trailing space in the header
		{"a: {int, min: 100}\n---\n42", true},
		{"a: {int, min: 1}\n---\n42", false},
	} {
		_, err := io.Parse(tc.src)
		if got := err != nil; got != tc.wantErr {
			t.Errorf("%q: error=%v, want error=%v (%v)", tc.src, got, tc.wantErr, err)
		}
	}
}

// A cached schema is shared across goroutines, so it must be read-only. Run
// under -race to mean anything.
func TestHeaderCacheConcurrent(t *testing.T) {
	type row struct {
		Name string `io:"name"`
		Age  int    `io:"age"`
	}
	const src = "name: string, age: int\n---\n~ Alice, 30\n~ Bob, 41"

	var wg sync.WaitGroup
	errc := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				var out []row
				if err := io.Unmarshal(src, &out); err != nil {
					errc <- err
					return
				}
				if len(out) != 2 || out[0].Name != "Alice" || out[1].Age != 41 {
					errc <- fmt.Errorf("goroutine %d got %+v", n, out)
					return
				}
				// A different header each time, to exercise the store path
				// concurrently with the load path.
				var one struct {
					V int `io:"v"`
				}
				if err := io.Unmarshal(fmt.Sprintf("v: int\n---\n%d", n*100+j), &one); err != nil {
					errc <- err
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatal(err)
	}
}

// Past its entry bound the cache stops storing and every document still
// decodes correctly — the bound degrades performance, never correctness.
func TestHeaderCacheBeyondItsBound(t *testing.T) {
	for i := 0; i < 1200; i++ {
		src := fmt.Sprintf("m%d: int\n---\n%d", i, i)
		doc, err := io.Parse(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		if got := doc.String(); !strings.HasSuffix(got, fmt.Sprint(i)) {
			t.Fatalf("%q round-tripped as %q", src, got)
		}
	}
	// A header larger than the cacheable bound is never stored, and still works.
	var big strings.Builder
	for i := 0; big.Len() < 5<<10; i++ {
		fmt.Fprintf(&big, "member_with_a_long_name_%d?: string, ", i)
	}
	src := strings.TrimSuffix(big.String(), ", ") + "\n---\n~ x"
	if _, err := io.Parse(src); err != nil {
		t.Fatalf("oversized header: %v", err)
	}
}

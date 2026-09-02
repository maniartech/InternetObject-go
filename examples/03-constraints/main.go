// Constraints in the `schema` tag — the format's own annotation syntax,
// verbatim. Marshal validates automatically (it can never emit a document
// that fails its own header); Validate runs the same check on demand after
// mutations; SchemaFor exposes the derived schema.
package main

import (
	"fmt"

	io "github.com/maniartech/InternetObject-go"
)

type User struct {
	Name string `io:"name" schema:"{string, minLen: 2, maxLen: 50}"`
	Age  int    `io:"age"  schema:"int, min: 0, max: 130"` // braces optional
	Role string `io:"role,omitempty" schema:"{string, choices: [admin, user]}"`
}

func main() {
	// The derived schema is a first-class value.
	s, err := io.SchemaFor[User]()
	if err != nil {
		panic(err)
	}
	fmt.Println("schema:", s.String())

	// Marshal auto-validates: an invalid value never reaches the wire.
	if _, err := io.Marshal(User{Name: "A", Age: 300}); err != nil {
		fmt.Println("marshal refused:", err) // mismatched-min-len; mismatched-max
	}

	// Validate on demand — the mutation story: mutate freely, check when it
	// matters (and the boundary re-checks regardless).
	u := User{Name: "Alice", Age: 30, Role: "admin"}
	fmt.Println("valid:", io.Validate(u) == nil)
	u.Age = -5
	fmt.Println("after mutation:", io.Validate(&u)) // mismatched-min

	// Constraints ride in the header, so the WIRE enforces them too.
	u.Age = 30
	text, _ := io.Marshal(u)
	var back User
	fmt.Println("wire-side:", io.Unmarshal(
		// simulate a tampered document
		text[:len(text)-len("Alice, 30, admin")]+"Alice, 131, admin", &back))
}

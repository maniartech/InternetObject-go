// Example 06 — code generation. The schema file is the source of truth; the
// guarded type is generated from it and cannot be put into an invalid state.
package main

import "fmt"

//go:generate go run github.com/maniartech/InternetObject-go/cmd/iogen -schema person.io -type Person

func main() {
	p, err := NewPerson("Alice", 30, "alice@example.com", true, 99.5, []string{"admin"})
	if err != nil {
		panic(err)
	}
	text, err := p.Marshal()
	if err != nil {
		panic(err)
	}
	fmt.Println(text)

	// The guard: a setter the schema rejects changes nothing.
	if err := p.SetAge(-1); err != nil {
		fmt.Println("rejected, as it should be:", err)
	}
	fmt.Println("age is still", p.Age())
}

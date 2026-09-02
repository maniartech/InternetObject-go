# Examples

Runnable tours of the API, smallest first. Each directory is a standalone program:

```bash
go run ./examples/01-parse
```

| Example | Shows | Status |
| ------- | ----- | ------ |
| [01-parse](01-parse/main.go) | Parse, accumulated errors, the value model, canonical writing | shipped |
| [02-structs](02-structs/main.go) | Marshal/Unmarshal with `io` tags; `optional` vs `omitempty`; pointers = nullable; nesting | shipped |
| [03-constraints](03-constraints/main.go) | `schema` tags, auto-validating Marshal, `Validate` after mutation, `SchemaFor` | shipped |
| [04-streaming](04-streaming/main.go) | `Stream`: incremental records, recoverable row faults, chunk independence | shipped |
| [05-runtime-schema](05-runtime-schema/main.go) | Attaching a schema at RUNTIME (fetched/stored elsewhere) — no tags involved | shipped |
| [PROPOSED.md](PROPOSED.md) | The full native surface from [ADR 0004](../docs/decisions/0004-native-api-design.md): embeddable bases, documents/sections/collections, typed definitions, `With` functions, `iogen` | design |

## The design in one idea

**A gradient, not a framework.** Plain structs and package functions always work; embedded
bases add method syntax; generated code adds unbypassable typed setters. Same engine, same
designated error codes, same wire text at every level — see ADR 0004.

## Schemas: three sources, one authority

1. **Design-time**: `schema:"{int, min: 0}"` struct tags (example 03).
2. **In the document**: the header the wire carries (examples 01, 02).
3. **Runtime**: fetched from a registry/file/service and attached dynamically (example 05).

Precedence when more than one is present: explicitly attached > document header >
tag-derived. `io` tags always remain the name-binding contract.

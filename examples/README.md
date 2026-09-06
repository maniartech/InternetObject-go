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
| [05-runtime-schema](05-runtime-schema/main.go) | Runtime schemas: compile once with `ParseSchema`, then `UnmarshalWith` / `ValidateWith` / `MarshalWith` / `ParseWith` / `StreamOptions.Schema` | shipped |
| [07-dashboard](07-dashboard/main.go) | **One request, several entity types.** A document carrying employees, alerts and stats, each section bound to its own schema; the receiver takes each one typed with `io.SectionAs[T]` | shipped |
| [06-codegen](06-codegen/main.go) | **Experimental.** `iogen` generates a guarded type from a schema file: unexported fields, constructor, typed setters — a value that exists is one the schema accepted | experimental |

`iogen` covers a single record against a single schema. Collections and multi-section documents
are not supported yet, so the generated shape may change ([ADR 0010](../docs/decisions/0010-code-generation.md)).
Nothing in the library depends on the tool, so this carries no risk for the stable API.

[ADR 0004](../docs/decisions/0004-native-api-design.md) designs a further native surface —
embeddable bases, documents/sections/collections, typed definitions and `iogen` code
generation. **None of it is implemented**; the five examples above are the whole API today.

## One verb per direction

`Marshal`/`Unmarshal` convert Go values ⇄ IO text (as in `encoding/json`); `Parse`/`String`
convert IO text ⇄ this module's own `Document` and `Schema` (as in `url.Parse`);  `Stream`
reads incrementally; `Validate` checks without producing text. `…With(…, s)` runs any of them
against an explicit schema. No `Load`, `Read`, `Decode` or `Write` anywhere — see
[ADR 0004 D0](../docs/decisions/0004-native-api-design.md).

## The design in one idea

**A gradient, not a framework.** Plain structs and package functions always work; embedded
bases add method syntax; generated code adds unbypassable typed setters. Same engine, same
designated error codes, same wire text at every level — see ADR 0004.

## Schemas: three sources, one authority

1. **Design-time**: `schema:"{int, min: 0}"` struct tags (example 03).
2. **In the document**: the header the wire carries (examples 01, 02).
3. **Runtime**: fetched from a registry/file/service (or lifted from another document with
   `doc.SchemaOf`), compiled once, and passed to the `With` functions (example 05).

Precedence when more than one is present: explicitly attached > document header >
tag-derived. `io` tags always remain the name-binding contract.

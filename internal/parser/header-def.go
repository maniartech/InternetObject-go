package parser

// HeaderDef is one header definition, with its sigil-less key.
type HeaderDef struct {
	Kind  DefKind
	Key   string
	Value any
}

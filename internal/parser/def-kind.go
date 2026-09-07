package parser

// DefKind classifies a header definition.
type DefKind uint8

const (
	DefPlain DefKind = iota
	DefSchema
	DefVar
)

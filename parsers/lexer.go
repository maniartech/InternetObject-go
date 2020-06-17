package parsers

/**
 * The lexer represents a class that lexical operations.
 */
type lexer struct {
	text   string
	length int
	tokens []Token
	done   bool

	// Current pos
	index int
	col   int8
	row   int8
}

/**
 * NewLexer initializes the new Lexer object.
 */
func NewLexer(text string) *lexer {
	l := new(lexer)

	l.text = text
	l.length = len(text)
	l.tokens = make([]Token, 0)
	l.done = false

	l.index = -1
	l.col = 0
	l.row = 0

	return l
}

/**
 * Read and parse the next token.
 */
func (l *lexer) Read() {

}

/**
 * ReadAll reads all the tokens.
 */
func (l *lexer) ReadAll() {

}

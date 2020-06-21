package parsers

/**
 * Token reprents the single token in the
 */
type Token struct {
	Text  string
	val   interface{}
	Type  string
	Start int
	End   int
	Row   int
	Col   int
}

/**
 * NewToken initializes the new instance of Token
 */
func NewToken(
	text string, val interface{}, tokenType string,
	start int, end int,
	row int, col int) *Token {

	t := new(Token)
	t.Text = text
	t.val = val
	t.Type = tokenType
	t.Start = start
	t.End = end
	t.Row = row
	t.Col = col

	return t

}

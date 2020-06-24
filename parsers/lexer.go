package parsers

import (
	"strconv"
	"strings"
)

type scanner func(l *lexer, start, end int) bool

/**
 * The lexer represents a class that lexical operations.
 */
type lexer struct {
	text   []rune
	length int
	tokens []*Token
	done   bool

	// Current pos
	ch    rune
	index int
	col   int
	row   int
}

/**
 * NewLexer initializes the new Lexer object.
 */
func NewLexer(text string) *lexer {
	l := new(lexer)

	l.text = []rune(text)
	l.length = len(text)
	l.tokens = make([]*Token, 0)
	l.done = false

	l.ch = rune(0)
	l.index = -1
	l.col = 0
	l.row = 1
	l.advance(1)
	return l
}

/**
 * ReadAll reads all the tokens.
 */
func (l *lexer) ReadAll() {
	for l.done != true {
		l.Read()
	}
}

/**
 * Read and parse the next token.
 */
func (l *lexer) Read() *Token {

	if l.done {
		return nil
	}

	var token *Token = nil

	// Validators
	datasep := false
	if l.ch == Hyphen {
		datasep = isDatasep(l)
	}

	// Scanners
	// Is separator
	if isSeparator(l.ch) {
		token = getToken(l, TypeSeparator, l.index, l.index)
		l.advance(1)
	} else if datasep {
		token = getToken(l, TypeDatasep, l.index, l.index+2)
		l.advance(3)
	} else {
		token = l.scan(TypeString, sepScanner, false)
		makeSenseIt(token)
	}
	if token != nil {
		l.tokens = append(l.tokens, token)
		return token
	}
	return nil
}

func (l *lexer) advance(times int) bool {

	if l.index+1 < l.length {
		l.index++
		l.col++
		l.ch = l.text[l.index]

		if l.ch == NewLine {
			l.col = 1
			l.row = 1
		}

		advanced := 1
		result := true
		for advanced < times {
			result = l.advance(1)
			advanced++
		}
		return result
	}

	l.ch = rune(0)
	l.done = true
	l.length = len(l.text) - 1
	return false
}

func (l *lexer) scan(tokenType string, scanner scanner, confined bool) *Token {
	start := -1

	if !isWS(l.ch) {
		start = l.index
	}

	for l.advance(1) {
		if start == -1 && !isWS(l.ch) {
			start = l.index
		}

		// Reached the end of the text, break it
		if l.done {
			break
		}

		if !scanner(l, start, l.index) {
			break
		}
	}

	if start == -1 {
		return nil
	}

	end := l.index
	if confined || l.done {
		end++
	}
	token := strings.TrimSpace(string(l.text[start:end]))
	tokenLen := len(token)

	if tokenLen == 0 {
		return nil
	}

	return NewToken(token, token, tokenType, start, start+tokenLen-1, l.row, l.col)
}

func getToken(l *lexer, tokenType string, start, end int) *Token {
	text := string(l.text[start : end+1])

	token := NewToken(
		text, text, tokenType, start, end, l.row, l.col)
	return token
}

func sepScanner(l *lexer, start, end int) bool {
	if isSeparator(l.ch) {
		return false
	}

	if l.ch == Hash {
		return false
	}

	if l.ch == Hyphen {
		return !isDatasep(l)
	}
	return true
}

func makeSenseIt(token *Token) {
	text := token.Text
	if text == "T" || text == "true" {
		token.Val = true
		token.Type = TypeBool
	}

	if text == "F" || text == "false" {
		token.Val = false
		token.Type = TypeBool
	}

	if text == "N" || text == "null" {
		token.Val = nil
		token.Type = TypeNull
	}

	if ReNumber.MatchString(text) {
		val, e := strconv.ParseFloat(text, 64)
		if e == nil {
			token.Val = val
			token.Type = TypeNumber
		}
	}
}

func isDatasep(l *lexer) bool {
	start := l.index
	end := l.index + 3

	if l.length < end {
		return false
	}
	return string(l.text[start:end]) == Datasep
}

func isSeparator(r rune) bool {
	return strings.ContainsRune(Separators, r)
}

func isWS(r rune) bool {
	return r <= Space
}

func isEndOfLine(r rune) bool {
	return r == NewLine || r == CarrigeReturn
}

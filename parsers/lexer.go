package parsers

import (
	"errors"
	"strconv"
	"strings"
)

type scanner func(l *lexer, start, end int) (bool, error)

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
func (l *lexer) ReadAll() error {
	for l.done != true {
		_, err := l.Read()

		if err != nil {
			return err
		}
	}
	return nil
}

/**
 * Read and parse the next token.
 */
func (l *lexer) Read() (*Token, error) {

	var token *Token = nil
	var err error = nil
	var advance int

	if l.done {
		return nil, err
	}

	// Validators
	datasep := false
	if l.ch == Hyphen {
		datasep = isDatasep(l)
	}

	// Scanners
	// Is separator
	if isWS(l.ch) {
		l.scan("ws", wsScanner, false)
	} else if isSeparator(l.ch) {
		token = getToken(l, TypeSeparator, l.index, l.index)
		advance = 1
	} else if l.ch == DoubleQuote {
		token, err = l.scan(TypeString, stringScanner, true)
		advance = 1
	} else if l.ch == Quote {
		token, err = l.scan("raw-string", rawStringScanner, true)
		advance = 1
	} else if datasep {
		token = getToken(l, TypeDatasep, l.index, l.index+2)
		advance = 3
	} else {
		token, err = l.scan(TypeString, sepScanner, false)
		makeSenseOfIt(token)
	}

	if err != nil {
		return nil, nil
	}

	if advance != 0 {
		l.advance(advance)
	}

	if token != nil {
		l.tokens = append(l.tokens, token)
	}

	return token, err
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

func (l *lexer) scan(tokenType string, scanner scanner, confined bool) (*Token, error) {
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

		continueScan, err := scanner(l, start, l.index)
		if err != nil {
			return nil, err
		}
		if !continueScan {
			break
		}
	}

	if start == -1 {
		return nil, nil
	}

	end := l.index
	if confined || l.done {
		end++
	}
	token := strings.TrimSpace(string(l.text[start:end]))
	tokenLen := len(token)

	if tokenLen == 0 {
		return nil, nil
	}

	return NewToken(token, token, tokenType, start, start+tokenLen-1, l.row, l.col), nil
}

func getToken(l *lexer, tokenType string, start, end int) *Token {
	text := string(l.text[start : end+1])

	token := NewToken(
		text, text, tokenType, start, end, l.row, l.col)
	return token
}

func wsScanner(l *lexer, start, end int) (bool, error) {
	return isWS(l.ch), nil
}

func sepScanner(l *lexer, start, end int) (bool, error) {
	if isSeparator(l.ch) {
		return false, nil
	}

	if l.ch == Hash {
		return false, nil
	}

	if l.ch == Hyphen {
		return !isDatasep(l), nil
	}
	return true, nil
}

func rawStringScanner(l *lexer, start, end int) (bool, error) {

	if l.ch != Quote {
		if l.index == l.length-1 {
			// incompelte-string
			return false, errors.New("syntax-error")
		}
		return true, nil
	}

	nextCh, e := getNexCh(l)

	// The current quote is a last char, stop scan.
	if e != nil {
		return false, nil
	}

	// The nextCh is a quote. It is escaping, cotinue scan.
	if nextCh == Quote {
		return true, nil
	}

	text := string(l.text[l.index : l.index+1])
	return ReRawString.MatchString(text), nil
}

func stringScanner(l *lexer, start, end int) (bool, error) {

	var err error = nil

	if l.ch != DoubleQuote {
		if l.index == l.length-1 {
			err = errors.New("syntax-error")
		}
		return true, err
	}
	return ReRegularString.MatchString(string(l.text[start : l.index+1])), err
}

func makeSenseOfIt(token *Token) {
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

func getNexCh(l *lexer) (rune, error) {
	// TODO: check this
	if l.index >= l.length-1 {
		return 0, errors.New("syntax-error")
	}
	return l.text[l.index+1], nil
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

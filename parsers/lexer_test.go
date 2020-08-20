package parsers

import (
	"testing"

	"github.com/maniartech/InternetObject-go/utils"
)

func TestLexer(t *testing.T) {
	l := NewLexer(`
	a, b, c, d
	---
	+1, -2.3, 2.3e+1000, '1400'
	`)
	e := l.ReadAll()
	if e != nil {
		println(e.Error())
	} else {
		utils.PrettyPrint(l.tokens)
	}
}

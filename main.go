package main

import (
	"github.com/maniartech/InternetObject-go/parsers"
	"github.com/maniartech/InternetObject-go/utils"
)

func main() {
	str := `name, age, "Hello\nWorld"`
	tokenizer := parsers.NewTokenizer(str)
	tokens, err := tokenizer.Tokenize()

	if err != nil {
		panic(err)
	}

	parser := parsers.NewParser(tokens)
	ast, err := parser.Parse()
	if err != nil {
		panic(err)
	}

	utils.PrettyPrint(ast)
}

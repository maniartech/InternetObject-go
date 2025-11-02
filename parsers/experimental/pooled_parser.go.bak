package parsers

// ParseStringParallel parses input using parallel tokenization for better performance on large inputs
func ParseStringParallel(input string) (*DocumentNode, error) {
	// Use parallel tokenizer for large inputs (>1KB)
	if len(input) > 1000 {
		pt := NewParallelTokenizer()
		tokens, err := pt.Tokenize(input)
		if err != nil {
			return nil, err
		}

		parser := NewParser(tokens)
		return parser.Parse()
	}

	// Use regular parsing for smaller inputs
	return ParseString(input)
}

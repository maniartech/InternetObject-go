package parsers

import (
	"runtime"
	"sync"
)

// ParallelTokenizer uses worker pools for parallel tokenization
type ParallelTokenizer struct {
	workers int
}

// NewParallelTokenizer creates a new parallel tokenizer
func NewParallelTokenizer() *ParallelTokenizer {
	return &ParallelTokenizer{
		workers: runtime.NumCPU(),
	}
}

// TokenizeChunk represents a chunk of input to tokenize
type TokenizeChunk struct {
	input    string
	startPos int
	startRow int
	startCol int
	tokens   []*Token
	err      error
}

// Tokenize tokenizes input using parallel workers
func (pt *ParallelTokenizer) Tokenize(input string) ([]*Token, error) {
	inputLen := len(input)

	// For small inputs, use sequential tokenization
	if inputLen < 1000 {
		t := NewTokenizer(input)
		return t.Tokenize()
	}

	// Split input into logical chunks (by newlines for better parallelization)
	chunks := pt.splitIntoChunks(input)

	if len(chunks) == 1 {
		// Single chunk, use sequential
		t := NewTokenizer(input)
		return t.Tokenize()
	}

	// Process chunks in parallel
	results := make([]*TokenizeChunk, len(chunks))
	var wg sync.WaitGroup
	errChan := make(chan error, len(chunks))

	// Use worker pool
	numWorkers := pt.workers
	if numWorkers > len(chunks) {
		numWorkers = len(chunks)
	}

	chunkChan := make(chan int, len(chunks))
	for i := range chunks {
		chunkChan <- i
	}
	close(chunkChan)

	// Start workers
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range chunkChan {
				chunk := chunks[idx]
				t := NewTokenizer(chunk.input)
				tokens, err := t.Tokenize()

				if err != nil {
					errChan <- err
					return
				}

				// Adjust token positions
				for _, token := range tokens {
					token.Position.Start.Pos += chunk.startPos
					token.Position.Start.Row += chunk.startRow
					if token.Position.Start.Row == chunk.startRow {
						token.Position.Start.Col += chunk.startCol
					}

					token.Position.End.Pos += chunk.startPos
					token.Position.End.Row += chunk.startRow
					if token.Position.End.Row == chunk.startRow {
						token.Position.End.Col += chunk.startCol
					}
				}

				chunk.tokens = tokens
				results[idx] = chunk
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return nil, err
	}

	// Merge results
	totalTokens := 0
	for _, chunk := range results {
		if chunk != nil {
			totalTokens += len(chunk.tokens)
		}
	}

	merged := make([]*Token, 0, totalTokens)
	for _, chunk := range results {
		if chunk != nil {
			merged = append(merged, chunk.tokens...)
		}
	}

	return merged, nil
}

// splitIntoChunks splits input into logical chunks for parallel processing
func (pt *ParallelTokenizer) splitIntoChunks(input string) []*TokenizeChunk {
	inputLen := len(input)

	// Target chunk size (rough estimate)
	targetChunkSize := inputLen / pt.workers
	if targetChunkSize < 500 {
		targetChunkSize = 500
	}

	chunks := make([]*TokenizeChunk, 0, pt.workers)

	pos := 0
	row := 0
	col := 0
	chunkStart := 0
	chunkStartRow := 0
	chunkStartCol := 0

	for pos < inputLen {
		ch := input[pos]

		// Look for section boundaries (---) or large enough chunks
		if ch == '\n' {
			chunkSize := pos - chunkStart

			// Check if we have a good chunk size or found section boundary
			if chunkSize >= targetChunkSize {
				// Check if next line starts with --- (section boundary)
				nextLineStart := pos + 1
				isSectionBoundary := false

				if nextLineStart+3 < inputLen {
					if input[nextLineStart] == '-' &&
						input[nextLineStart+1] == '-' &&
						input[nextLineStart+2] == '-' {
						isSectionBoundary = true
					}
				}

				if isSectionBoundary || chunkSize >= targetChunkSize*2 {
					// Create chunk
					chunks = append(chunks, &TokenizeChunk{
						input:    input[chunkStart : pos+1],
						startPos: chunkStart,
						startRow: chunkStartRow,
						startCol: chunkStartCol,
					})

					// Start new chunk
					chunkStart = pos + 1
					chunkStartRow = row + 1
					chunkStartCol = 0
				}
			}

			row++
			col = 0
		} else {
			col++
		}

		pos++
	}

	// Add final chunk
	if chunkStart < inputLen {
		chunks = append(chunks, &TokenizeChunk{
			input:    input[chunkStart:],
			startPos: chunkStart,
			startRow: chunkStartRow,
			startCol: chunkStartCol,
		})
	}

	return chunks
}

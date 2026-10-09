package parser

import "github.com/nooga/paserati/pkg/lexer"

// tokenChunkSize sets how many Tokens live in a single pool chunk.
// Chunks are never resized, which keeps pointers into them stable even
// as the pool grows.
const tokenChunkSize = 1024

// TokenPool is a chunk-allocated pool of stable *lexer.Token pointers.
// Parsed tokens are copied into a chunk; the returned pointer remains
// valid for the life of the pool. This lets AST nodes reference tokens
// by pointer (8 bytes) instead of embedding the full Token struct.
type TokenPool struct {
	chunks [][]lexer.Token
}

// NewTokenPool creates an empty pool with one pre-allocated chunk.
func NewTokenPool() *TokenPool { return newTokenPoolSized(tokenChunkSize) }

// newTokenPoolSized creates a pool whose first chunk holds about the given
// number of tokens (at least 16, at most a full chunk). Later chunks double
// in size up to a full chunk, so the pool never holds much more capacity than
// tokens: a module that keeps its AST should not retain a thousand empty
// Tokens (each is over a hundred bytes) for a few dozen lines of source.
func newTokenPoolSized(tokens int) *TokenPool {
	if tokens > tokenChunkSize {
		tokens = tokenChunkSize
	}
	if tokens < 16 {
		tokens = 16
	}
	return &TokenPool{
		chunks: [][]lexer.Token{make([]lexer.Token, 0, tokens)},
	}
}

// Take copies t into the current chunk (allocating a new chunk if the
// current one is full) and returns a stable pointer to the stored Token.
func (p *TokenPool) Take(t lexer.Token) *lexer.Token {
	cur := &p.chunks[len(p.chunks)-1]
	if len(*cur) == cap(*cur) {
		// Chunks double up to the full size, so a source that outgrows its
		// estimate does not jump straight to a thousand empty Tokens.
		next := 2 * cap(*cur)
		if next > tokenChunkSize {
			next = tokenChunkSize
		}
		p.chunks = append(p.chunks, make([]lexer.Token, 0, next))
		cur = &p.chunks[len(p.chunks)-1]
	}
	*cur = append(*cur, t)
	return &(*cur)[len(*cur)-1]
}

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
func NewTokenPool() *TokenPool {
	return &TokenPool{
		chunks: [][]lexer.Token{make([]lexer.Token, 0, tokenChunkSize)},
	}
}

// Take copies t into the current chunk (allocating a new chunk if the
// current one is full) and returns a stable pointer to the stored Token.
func (p *TokenPool) Take(t lexer.Token) *lexer.Token {
	cur := &p.chunks[len(p.chunks)-1]
	if len(*cur) == cap(*cur) {
		p.chunks = append(p.chunks, make([]lexer.Token, 0, tokenChunkSize))
		cur = &p.chunks[len(p.chunks)-1]
	}
	*cur = append(*cur, t)
	return &(*cur)[len(*cur)-1]
}

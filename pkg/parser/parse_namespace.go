package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/lexer"
)

// parseNamespaceDeclaration parses:
//
//	namespace X { ... }
//	namespace A.B.C { ... }
//	declare namespace X { ... }
//
// On entry curToken is the contextual `namespace` keyword (IDENT). On exit
// curToken is the closing `}` of the body (so the surrounding statement loop
// can advance normally), matching how block-bodied statements end.
func (p *Parser) parseNamespaceDeclaration(declare bool) *NamespaceDeclaration {
	nsToken := p.curToken // 'namespace'

	// TS1038: a 'declare' modifier cannot be used in an already ambient context.
	// (`declare namespace` introduces ambient context for its body, so a nested
	// `declare namespace` is doubly redundant.)
	if declare && p.inAmbientContext > 0 {
		p.addError(nsToken, "A 'declare' modifier cannot be used in an already ambient context.")
	}

	// Move to first name segment
	p.nextToken()
	if !p.curTokenIsIdentLike() {
		p.addError(p.curToken, fmt.Sprintf("expected namespace name, got %s", p.curToken.Type))
		return nil
	}

	// Collect dotted name segments
	type seg struct {
		tok   *lexer.Token
		value string
	}
	segs := []seg{{tok: p.curToken, value: p.curToken.Literal}}
	for p.peekTokenIs(lexer.DOT) {
		p.nextToken() // consume '.'
		p.nextToken() // move to next segment
		if !p.curTokenIsIdentLike() {
			p.addError(p.curToken, fmt.Sprintf("expected namespace name segment after '.', got %s", p.curToken.Type))
			return nil
		}
		segs = append(segs, seg{tok: p.curToken, value: p.curToken.Literal})
	}

	// Expect '{'
	if !p.expectPeek(lexer.LBRACE) {
		return nil
	}

	// `declare namespace X { ... }` makes the body an ambient context — nested
	// `declare` modifiers in it are TS1038 errors. The same applies to a plain
	// `namespace X { ... }` whose body declares functions without bodies, but
	// per tsc the ambient flag is set only by `declare`.
	if declare {
		p.inAmbientContext++
	}
	body := p.parseBlockStatement()
	if declare {
		p.inAmbientContext--
	}
	if body == nil {
		return nil
	}

	// Build innermost namespace (segs[len-1]) wrapping the body
	innermost := &NamespaceDeclaration{
		Token:   nsToken,
		Name:    &Identifier{Token: segs[len(segs)-1].tok, Value: segs[len(segs)-1].value},
		Body:    body,
		Declare: declare,
	}

	// Wrap each outer segment around the inner one. Each intermediate segment
	// implicitly exports the next segment so that A.B.x is reachable via A.B.
	current := innermost
	for i := len(segs) - 2; i >= 0; i-- {
		// Inner namespace must be exported from its enclosing one.
		current.IsExported = true
		// Build a synthetic block statement containing the inner namespace.
		inner := current
		wrapBlock := &BlockStatement{
			Token:      nsToken,
			Statements: []Statement{inner},
		}
		current = &NamespaceDeclaration{
			Token:   nsToken,
			Name:    &Identifier{Token: segs[i].tok, Value: segs[i].value},
			Body:    wrapBlock,
			Declare: declare,
		}
	}

	return current
}

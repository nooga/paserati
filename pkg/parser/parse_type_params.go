package parser

import (
	"strings"

	"github.com/nooga/paserati/pkg/lexer"
)

// isTypeParamNameToken reports whether tok can be the name of a parameter in a
// function type / signature. TypeScript accepts any identifier name here,
// including contextual keywords such as `type`, `from` and `set`.
func (p *Parser) isTypeParamNameToken(tok *lexer.Token) bool {
	if tok.Type == lexer.IDENT {
		return true
	}
	if tok.Type == lexer.THIS {
		return true
	}
	return p.isKeywordThatCanBeIdentifier(tok.Type)
}

// scanBindingPatternThenColon is called with cur at '[' or '{'. It looks ahead
// (without consuming anything) to the matching close bracket and reports
// whether it is followed by ':' or '?', i.e. whether the bracketed text is a
// binding pattern naming a parameter rather than a type.
func (p *Parser) scanBindingPatternThenColon() bool {
	savedCur, savedPeek := p.curToken, p.peekToken
	savedState := p.l.SaveState()
	savedErrs := len(p.errors)
	defer func() {
		p.curToken, p.peekToken = savedCur, savedPeek
		p.l.RestoreState(savedState)
		if len(p.errors) > savedErrs {
			p.errors = p.errors[:savedErrs]
		}
	}()
	depth := 0
	for !p.curTokenIs(lexer.EOF) {
		switch p.curToken.Type {
		case lexer.LBRACKET, lexer.LBRACE, lexer.LPAREN:
			depth++
		case lexer.RBRACKET, lexer.RBRACE, lexer.RPAREN:
			depth--
			if depth == 0 {
				return p.peekTokenIs(lexer.COLON) || p.peekTokenIs(lexer.QUESTION)
			}
		}
		p.nextToken()
	}
	return false
}

// skipBalancedPattern consumes a bracketed binding pattern; cur is at the
// opening bracket on entry and at the closing bracket on exit.
func (p *Parser) skipBalancedPattern() {
	depth := 0
	for !p.curTokenIs(lexer.EOF) {
		switch p.curToken.Type {
		case lexer.LBRACKET, lexer.LBRACE, lexer.LPAREN:
			depth++
		case lexer.RBRACKET, lexer.RBRACE, lexer.RPAREN:
			depth--
			if depth == 0 {
				return
			}
		}
		p.nextToken()
	}
}

// typeSigParams is the result of parsing the parameter list of a function
// type or method signature.
type typeSigParams struct {
	params []Expression
	names  []*Identifier // the named parameters, for duplicate checks
	// paramNames is the written name of each of params ("" when the list
	// gives only a type), then, when there is a rest parameter, its name.
	paramNames []string
	optional   []bool
	rest       Expression
	// this is the type of an explicit `this` parameter, which is not part of
	// the call signature.
	this Expression
	// bareName is set when the list consists of exactly one unannotated
	// identifier, e.g. `(Foo)`: that is a parenthesized type unless an arrow
	// follows.
	bareName Expression
}

// parseTypeSignatureParams parses `( ... )` in a function type or method
// signature. cur is '(' on entry and the closing ')' on exit. ok=false means an
// error was already recorded.
func (p *Parser) parseTypeSignatureParams() (res typeSigParams, ok bool) {
	if !p.curTokenIs(lexer.LPAREN) {
		p.addError(p.curToken, "expected '(' for parameter list")
		return res, false
	}
	defer func() {
		if ok && p.tscDiagnostics {
			reportDuplicateParams(p, res.names)
		}
	}()
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return res, true
	}
	p.nextToken() // first token of first parameter

	for {
		if p.curTokenIs(lexer.RPAREN) { // trailing comma
			return res, true
		}

		if p.curTokenIs(lexer.SPREAD) {
			restTok := p.curToken
			restName := ""
			if p.peekToken != nil {
				switch {
				case p.isTypeParamNameToken(p.peekToken):
					restName = p.peekToken.Literal
				case p.peekTokenIs(lexer.LBRACKET) || p.peekTokenIs(lexer.LBRACE):
					restName = "__pattern"
				}
			}
			rest := p.parseRestParameterType()
			if rest == nil {
				return res, false
			}
			if res.rest == nil {
				res.rest = rest
				res.paramNames = append(res.paramNames[:len(res.params):len(res.params)], restName)
			}
			if p.curTokenIs(lexer.RPAREN) {
				return res, true
			}
			if p.peekTokenIs(lexer.COMMA) && p.tscDiagnostics {
				// tsc parses on; its checker reports the misplaced rest.
				if p.canRecover() {
					p.addGrammarErrorWithCode(restTok, "TS1014", "A rest parameter must be last in a parameter list.")
				}
				p.nextToken()
				p.nextToken()
				continue
			}
			if !p.expectPeek(lexer.RPAREN) {
				return res, false
			}
			return res, true
		}

		if p.tscDiagnostics {
			// tsc parses parameter-property modifiers here too and refuses
			// them in the checker.
			var modTok *lexer.Token
			// Only before a plain name: `(readonly [K, V])` is a parenthesized
			// readonly tuple type, not a parameter.
			for (p.curTokenIs(lexer.PUBLIC) || p.curTokenIs(lexer.PRIVATE) || p.curTokenIs(lexer.PROTECTED) || p.curTokenIs(lexer.READONLY) || p.curTokenIs(lexer.OVERRIDE)) &&
				(p.peekTokenIs(lexer.IDENT) || p.isTypeParamNameToken(p.peekToken)) {
				if modTok == nil {
					modTok = p.curToken
				}
				p.nextToken()
			}
			if modTok != nil && p.canRecover() {
				p.addSemanticErrorWithCode(modTok, "TS2369", "A parameter property is only allowed in a constructor implementation.")
			}
		}
		bare := false
		optional := false
		name := ""
		switch {
		case p.curTokenIs(lexer.THIS) && p.peekTokenIs(lexer.COLON):
			// `this: T` is not part of the call signature; keep it apart.
			p.nextToken()
			p.nextToken()
			res.this = p.parseTypeExpression()
			if res.this == nil {
				return res, false
			}
			goto next
		case (p.curTokenIs(lexer.LBRACKET) || p.curTokenIs(lexer.LBRACE)) && p.scanBindingPatternThenColon():
			name = p.bindingPatternText()
			if p.peekTokenIs(lexer.QUESTION) {
				p.nextToken()
				optional = true
			}
			if !p.expectPeek(lexer.COLON) {
				return res, false
			}
			p.nextToken()
		case p.isTypeParamNameToken(p.curToken):
			isIdent := p.curTokenIs(lexer.IDENT)
			if p.peekTokenIs(lexer.COLON) || p.peekTokenIs(lexer.QUESTION) || (isIdent && (p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RPAREN))) {
				name = p.curToken.Literal
			}
			if p.peekTokenIs(lexer.COLON) || p.peekTokenIs(lexer.QUESTION) || (isIdent && (p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RPAREN))) {
				res.names = append(res.names, &Identifier{Token: p.curToken, Value: p.curToken.Literal})
			}
			if p.peekTokenIs(lexer.COLON) {
				p.nextToken()
				p.nextToken()
			} else if p.peekTokenIs(lexer.QUESTION) && (isIdent || p.curToken.Type != lexer.THIS) {
				p.nextToken() // name -> '?'
				optional = true
				if p.peekTokenIs(lexer.COLON) {
					p.nextToken()
					p.nextToken()
				} else if p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RPAREN) {
					// `(x?)`: optional, untyped (implicitly any)
					res.params = append(res.params, &Identifier{
						Token: &lexer.Token{Type: lexer.IDENT, Literal: "any"},
						Value: "any",
					})
					res.optional = append(res.optional, true)
					res.paramNames = append(res.paramNames, name)
					goto next
				} else {
					p.addError(p.peekToken, "':' expected.")
					return res, false
				}
			} else if isIdent && (p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.RPAREN)) {
				// bare name: an untyped (implicitly any) parameter
				bare = true
				res.bareName = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
				res.params = append(res.params, &Identifier{
					Token: &lexer.Token{Type: lexer.IDENT, Literal: "any"},
					Value: "any",
				})
				res.optional = append(res.optional, false)
				res.paramNames = append(res.paramNames, name)
				goto next
			}
		}
		_ = bare
		{
			t := p.parseTypeExpression()
			if t == nil {
				return res, false
			}
			res.params = append(res.params, t)
			res.optional = append(res.optional, optional)
			res.paramNames = append(res.paramNames, name)
		}
	next:
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			p.nextToken()
			continue
		}
		break
	}
	if len(res.params) != 1 || res.rest != nil {
		res.bareName = nil
	}
	if !p.expectPeek(lexer.RPAREN) {
		return res, false
	}
	return res, true
}

// bindingPatternText consumes a bracketed binding pattern naming a parameter
// (cur at the opening bracket on entry, at the closing one on exit) and
// returns it as tsc prints such a name: `{ a, b }`, `[x, y]`.
func (p *Parser) bindingPatternText() string {
	var b strings.Builder
	depth := 0
	prev := lexer.TokenType("")
	for !p.curTokenIs(lexer.EOF) {
		tok := p.curToken
		switch tok.Type {
		case lexer.RBRACE:
			if prev != lexer.LBRACE {
				b.WriteString(" ")
			}
		case lexer.COMMA, lexer.RBRACKET, lexer.RPAREN, lexer.COLON:
		default:
			switch prev {
			case "", lexer.LBRACKET, lexer.LPAREN, lexer.SPREAD:
			case lexer.LBRACE, lexer.COMMA, lexer.COLON, lexer.ASSIGN:
				b.WriteString(" ")
			default:
				if tok.Type == lexer.ASSIGN {
					b.WriteString(" ")
				}
			}
		}
		b.WriteString(tok.Literal)
		switch tok.Type {
		case lexer.LBRACKET, lexer.LBRACE, lexer.LPAREN:
			depth++
		case lexer.RBRACKET, lexer.RBRACE, lexer.RPAREN:
			depth--
		}
		prev = tok.Type
		if depth == 0 {
			return b.String()
		}
		p.nextToken()
	}
	return b.String()
}

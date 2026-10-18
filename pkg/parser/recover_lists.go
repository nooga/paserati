package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
)

// List-parsing recovery in the style of tsc's parseList: when the current
// token cannot start an element of the list being parsed, tsc reports the
// list's error and then either abandons the list (when the token makes sense
// to some enclosing construct, e.g. it starts a statement) or skips it.

// tokensAhead returns up to n tokens starting with the current one, without
// consuming anything.
func (p *Parser) tokensAhead(n int) []lexer.Token {
	toks := make([]lexer.Token, 0, n)
	toks = append(toks, *p.curToken)
	if n > 1 {
		toks = append(toks, *p.peekToken)
	}
	if n > 2 && p.peekToken.Type != lexer.EOF {
		p.scanAhead(1, func(tok lexer.Token) bool {
			toks = append(toks, tok)
			return len(toks) < n
		})
	}
	return toks
}

// tscContextualKeywords are words tsc's scanner makes keyword tokens (so
// isKeyword holds) that our lexer scans as plain identifiers.
var tscContextualKeywords = map[string]bool{
	"accessor": true, "any": true, "assert": true, "asserts": true, "bigint": true, "boolean": true,
	"constructor": true, "declare": true, "defer": true, "global": true, "intrinsic": true, "module": true,
	"namespace": true, "never": true, "number": true, "object": true, "out": true, "require": true,
	"string": true, "symbol": true, "unique": true, "unknown": true, "using": true,
}

// isTscKeyword reports whether tsc scans tok as a keyword.
func isTscKeyword(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.IDENT:
		return tscContextualKeywords[tok.Literal] && !tok.HasEscape
	case lexer.STRING, lexer.NUMBER, lexer.BIGINT, lexer.PRIVATE_IDENT:
		return false
	}
	_, ok := lexerKeywordTypes[tok.Type]
	return ok
}

var lexerKeywordTypes = func() map[lexer.TokenType]bool {
	m := map[lexer.TokenType]bool{}
	for _, word := range []string{"function", "let", "var", "const", "true", "false", "if", "else", "return", "null",
		"undefined", "while", "do", "for", "with", "break", "continue", "type", "switch", "case", "default", "this",
		"new", "interface", "extends", "implements", "super", "typeof", "void", "keyof", "infer", "is", "of", "as",
		"satisfies", "in", "instanceof", "delete", "try", "catch", "throw", "finally", "debugger", "class", "enum",
		"static", "readonly", "public", "private", "protected", "abstract", "override", "get", "set", "import",
		"export", "from", "yield", "async", "await"} {
		if t := lexer.LookupIdent(word); t != lexer.IDENT {
			m[t] = true
		}
	}
	return m
}()

// isIdentifierOrKeywordToken is tsc's tokenIsIdentifierOrKeyword.
func isIdentifierOrKeywordToken(tok *lexer.Token) bool {
	return tok.Type == lexer.IDENT || lexerKeywordTypes[tok.Type]
}

// isTscModifier is tsc's isModifierKind.
func isTscModifier(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.ABSTRACT, lexer.ASYNC, lexer.CONST, lexer.DEFAULT, lexer.EXPORT, lexer.IN, lexer.PRIVATE,
		lexer.PROTECTED, lexer.PUBLIC, lexer.READONLY, lexer.STATIC, lexer.OVERRIDE:
		return true
	case lexer.IDENT:
		return !tok.HasEscape && (tok.Literal == "accessor" || tok.Literal == "declare" || tok.Literal == "out")
	}
	return false
}

// isTscClassMemberModifier is tsc's isClassMemberModifier.
func isTscClassMemberModifier(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.PUBLIC, lexer.PRIVATE, lexer.PROTECTED, lexer.READONLY, lexer.STATIC, lexer.ABSTRACT, lexer.OVERRIDE:
		return true
	case lexer.IDENT:
		return !tok.HasEscape && tok.Literal == "accessor"
	}
	return false
}

// isLiteralPropertyNameToken is tsc's isLiteralPropertyName.
func isLiteralPropertyNameToken(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.STRING, lexer.NUMBER, lexer.BIGINT, lexer.PRIVATE_IDENT:
		return true
	}
	return isIdentifierOrKeywordToken(tok)
}

// isClassMemberStart is tsc's isClassMemberStart, judged from the current
// token on.
func (p *Parser) isClassMemberStart() bool {
	toks := p.tokensAhead(24)
	i := 0
	at := func(k int) *lexer.Token {
		if k < len(toks) {
			return &toks[k]
		}
		return &toks[len(toks)-1]
	}
	if at(0).Type == lexer.AT {
		return true
	}
	var idToken *lexer.Token
	for isTscModifier(at(i)) && i < len(toks)-1 {
		idToken = at(i)
		if isTscClassMemberModifier(idToken) {
			return true
		}
		i++
	}
	if at(i).Type == lexer.ASTERISK {
		return true
	}
	if isLiteralPropertyNameToken(at(i)) {
		idToken = at(i)
		i++
	}
	if at(i).Type == lexer.LBRACKET {
		return true
	}
	if idToken != nil {
		if !isTscKeyword(idToken) || idToken.Type == lexer.GET || idToken.Type == lexer.SET {
			return true
		}
		switch at(i).Type {
		case lexer.LPAREN, lexer.LT, lexer.BANG, lexer.COLON, lexer.ASSIGN, lexer.QUESTION:
			return true
		}
		next := at(i)
		return next.Type == lexer.SEMICOLON || next.Type == lexer.RBRACE || next.Type == lexer.EOF || next.Line > at(i-1).Line
	}
	return false
}

// listAbortsAt reports whether a token that cannot continue the current list
// ends it because an enclosing construct can use it (tsc's
// isInSomeParsingContext), approximated as "it can start a statement".
func listAbortsAt(tok *lexer.Token) bool {
	return tok.Type == lexer.EOF || tok.Type == lexer.RBRACE || !(cannotStartStatement(tok.Type) || tok.Type == lexer.SEMICOLON)
}

// recoverClassMember handles a token that cannot start a class member: it
// reports TS1068 and reports whether the class body should be abandoned at it
// (true) or whether the token was skipped (false).
func (p *Parser) recoverClassMember() bool {
	p.addErrorWithCode(p.curToken, "TS1068", "Unexpected token. A constructor, method, accessor, or property was expected.")
	if listAbortsAt(p.curToken) {
		return true
	}
	p.nextToken()
	return false
}

// missingCloseBrace reports "'}' expected." at the current token for a body
// that ended without one (tsc's parseExpected), deduplicated against an error
// already reported there.
func (p *Parser) missingCloseBrace() {
	p.addErrorWithCode(p.curToken, errors.TS1005, "'}' expected.")
}

// listStep says how a delimited list goes on after recovering from a missing
// delimiter.
type listStep int

const (
	listContinue listStep = iota // the next element starts at peek
	listClose                    // peek is the list's closing token
	listAbort                    // the list is abandoned; peek belongs to an enclosing construct
)

// recoverMissingComma is tsc's parseDelimitedList recovery after an element
// that is followed by neither ',' nor the closing token. cur is the element's
// last token. It reports "',' expected." at the next token, then skips tokens
// that neither start an element nor make sense to an enclosing construct.
func (p *Parser) recoverMissingComma(close lexer.TokenType) listStep {
	p.addErrorWithCode(p.peekToken, errors.TS1005, "',' expected.")
	for {
		switch {
		case p.peekTokenIs(close):
			return listClose
		case p.startsExpression(p.peekToken) || p.peekTokenIs(lexer.COMMA) || p.peekTokenIs(lexer.SPREAD):
			return listContinue
		case p.peekTokenIs(lexer.SEMICOLON) || listAbortsAt(p.peekToken):
			return listAbort
		}
		p.nextToken()
	}
}

// tscReservedWords are tsc's FirstReservedWord..LastReservedWord.
var tscReservedWords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true, "continue": true, "debugger": true,
	"default": true, "delete": true, "do": true, "else": true, "enum": true, "export": true, "extends": true,
	"false": true, "finally": true, "for": true, "function": true, "if": true, "import": true, "in": true,
	"instanceof": true, "new": true, "null": true, "return": true, "super": true, "switch": true, "this": true,
	"throw": true, "true": true, "try": true, "typeof": true, "var": true, "void": true, "while": true, "with": true,
}

// reportMissingBindingIdentifier is tsc's createIdentifier for a token that is
// not a binding identifier: TS1359 for a reserved word, TS1003 otherwise. The
// token is left unconsumed: on success cur is stepped back before it, so a
// caller that expects cur on the name's (zero-width) last token can go on.
func (p *Parser) reportMissingBindingIdentifier() bool {
	tok := p.curToken
	if tok.Type != lexer.IDENT && tscReservedWords[tok.Literal] {
		p.addErrorWithCode(tok, "TS1359", "Identifier expected. '"+tok.Literal+"' is a reserved word that cannot be used here.")
	} else {
		p.addErrorWithCode(tok, errors.TS1003, "Identifier expected.")
	}
	return p.stepBackForMissing()
}

// expectPeekParenRecover expects the '(' of a parameter list. When it is
// missing tsc reports it and treats the list as missing, so on recovery this
// reports and returns true without advancing; parseMethodParameters then
// yields an empty list.
func (p *Parser) expectPeekParenRecover() bool {
	if p.peekTokenIs(lexer.LPAREN) || !p.canRecover() {
		return p.expectPeek(lexer.LPAREN)
	}
	p.peekError(lexer.LPAREN)
	return true
}

// expectPeekBraceRecover is expectPeekParenRecover for a function body's '{';
// parseFunctionBody then yields an empty body.
func (p *Parser) expectPeekBraceRecover() bool {
	if p.peekTokenIs(lexer.LBRACE) || !p.canRecover() {
		return p.expectPeek(lexer.LBRACE)
	}
	p.peekError(lexer.LBRACE)
	return true
}

// startsParameter is tsc's isStartOfParameter, narrowed to the tokens that
// can begin a parameter's name, modifiers or decorators.
func (p *Parser) startsParameter(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.SPREAD, lexer.AT, lexer.LBRACKET, lexer.LBRACE, lexer.THIS, lexer.IDENT, lexer.PRIVATE_IDENT:
		return true
	}
	return isIdentifierOrKeywordToken(tok)
}

// parseParameterListRecover parses a parameter list (cur is the '(') with
// tsc's parseDelimitedList(Parameters) recovery: a missing ',' is reported and
// the next parameter parsed, a token that cannot start a parameter is
// reported (TS1138) and skipped, or ends the list when an enclosing construct
// can use it. On return cur is the ')' (synthesized if missing).
func (p *Parser) parseParameterListRecover(allowParameterProperties bool) ([]*Parameter, *RestParameter, error) {
	params := []*Parameter{}
	var rest *RestParameter
	first := true
	for !p.peekTokenIs(lexer.RPAREN) && !p.peekTokenIs(lexer.EOF) {
		if !p.startsParameter(p.peekToken) {
			p.addErrorWithCode(p.peekToken, "TS1138", "Parameter declaration expected.")
			if p.peekTokenIs(lexer.RBRACKET) || listAbortsAt(p.peekToken) {
				break
			}
			p.nextToken()
			continue
		}
		p.nextToken()
		var decorators []*Decorator
		if p.curTokenIs(lexer.AT) {
			decorators = p.parseDecoratorList()
		}
		if p.curTokenIs(lexer.SPREAD) {
			r := p.parseRestParameter()
			if r == nil {
				return nil, nil, fmt.Errorf("failed to parse rest parameter")
			}
			if p.peekTokenIs(lexer.QUESTION) {
				p.nextToken()
				p.addGrammarErrorWithCode(p.curToken, "TS1047", "A rest parameter cannot be optional.")
			}
			if p.peekTokenIs(lexer.ASSIGN) {
				if r.Name != nil {
					p.addGrammarErrorWithCode(r.Name.Token, "TS1048", "A rest parameter cannot have an initializer.")
				}
				p.nextToken()
				p.nextToken()
				p.parseExpression(COMMA)
			}
			if p.peekTokenIs(lexer.COMMA) {
				p.addGrammarErrorWithCode(r.Token, "TS1014", "A rest parameter must be last in a parameter list.")
			}
			if rest == nil {
				rest = r
			}
		} else {
			param, err := p.parseOneParameter(allowParameterProperties, first, decorators)
			if err != nil {
				return nil, nil, err
			}
			params = append(params, param)
		}
		first = false
		if p.peekTokenIs(lexer.COMMA) {
			p.nextToken()
			continue
		}
		if p.peekTokenIs(lexer.RPAREN) || p.peekTokenIs(lexer.RBRACKET) || p.peekTokenIs(lexer.EOF) {
			break
		}
		p.addErrorWithCode(p.peekToken, errors.TS1005, "',' expected.")
		if !p.startsParameter(p.peekToken) && listAbortsAt(p.peekToken) {
			break
		}
	}
	if !p.expectPeekRecover(lexer.RPAREN) {
		return nil, nil, fmt.Errorf("expected closing parenthesis after parameters")
	}
	return params, rest, nil
}

// arrowParamsAreDefinite reports whether a parenthesized list can only be
// arrow function parameters: it is empty, or it uses syntax no parenthesized
// expression has (a rest parameter, a type annotation, an optional marker).
func arrowParamsAreDefinite(params []*Parameter, rest *RestParameter) bool {
	if len(params) == 0 || rest != nil {
		return true
	}
	for _, prm := range params {
		if prm.TypeAnnotation != nil || prm.Optional {
			return true
		}
	}
	return false
}

// parseArrowParameterListTsc parses a parenthesized arrow parameter list (cur
// is the '(') the way tsc's parseParameters does, so that a list tsc accepts
// with grammar errors (a rest parameter that is not last) still reads as
// parameters. It fails, without recovery, on anything else; the caller is
// speculating.
func (p *Parser) parseArrowParameterListTsc() ([]*Parameter, *RestParameter, error) {
	params := []*Parameter{}
	var rest *RestParameter
	if p.peekTokenIs(lexer.RPAREN) {
		p.nextToken()
		return params, nil, nil
	}
	for first := true; ; first = false {
		p.nextToken()
		if p.curTokenIs(lexer.SPREAD) {
			r := p.parseRestParameter()
			if r == nil {
				return nil, nil, fmt.Errorf("failed to parse rest parameter")
			}
			if rest == nil {
				rest = r
				r.NotLast = p.peekTokenIs(lexer.COMMA)
			}
		} else {
			param, err := p.parseOneParameter(false, first, nil)
			if err != nil {
				return nil, nil, err
			}
			params = append(params, param)
		}
		if !p.peekTokenIs(lexer.COMMA) {
			break
		}
		p.nextToken()
		if p.peekTokenIs(lexer.RPAREN) {
			p.nextToken()
			return params, rest, nil
		}
	}
	if !p.expectPeek(lexer.RPAREN) {
		return nil, nil, fmt.Errorf("expected closing parenthesis after parameters")
	}
	return params, rest, nil
}

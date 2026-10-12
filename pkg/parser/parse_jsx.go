package parser

import (
	"strings"

	"github.com/nooga/paserati/pkg/lexer"
)

// JSX parsing. Inside JSX the token grammar differs from ordinary code (names
// may contain '-', strings have no escapes, text runs between tags), so the
// parser drives the lexer's JSX scanners itself: it rewinds the lexer to the
// end of the token it consumed (the ordinary lookahead after it is dropped),
// scans in JSX mode, and refills the lookahead when it hands back to the
// expression parser, for `{expr}` and after the element.

// isJSXFile reports whether a file of this name is parsed with JSX (tsc's
// languageVariant: .tsx and .jsx files).
func isJSXFile(name string) bool {
	return strings.HasSuffix(name, ".tsx") || strings.HasSuffix(name, ".jsx")
}

// jsxRewind positions the lexer just after p.curToken, dropping p.peekToken.
func (p *Parser) jsxRewind() {
	if p.prePeekCur == p.curToken {
		p.l.RestoreState(p.prePeek)
		return
	}
	t := p.curToken
	p.l.Seek(t.EndPos, t.Line, t.Column+t.EndPos-t.StartPos)
}

// jsxRefill scans p.peekToken as ordinary code from where the lexer stands.
func (p *Parser) jsxRefill() {
	p.prePeek = p.l.SaveState()
	p.prePeekCur = p.curToken
	p.peekToken = p.tokenPool.Take(p.l.NextToken())
}

// jsxConsume makes t the current token.
func (p *Parser) jsxConsume(t lexer.Token) *lexer.Token {
	tok := p.tokenPool.Take(t)
	if len(tok.Diags) > 0 {
		p.reportTokenDiags(tok.Diags)
	}
	p.prevToken = p.curToken
	p.curToken = tok
	return tok
}

func (p *Parser) jsxTagToken() *lexer.Token { return p.jsxConsume(p.l.ScanJSXTagToken()) }

// isJSXGenericArrowStart reports whether `<` (the current token) starts a
// generic arrow function rather than JSX: `<T,`, `<T =` or `<T extends X`
// (optionally `<const T ...`), as tsc decides in a JSX file.
func (p *Parser) isJSXGenericArrowStart() bool {
	if !p.peekTokenIs(lexer.IDENT) && !p.peekTokenIs(lexer.CONST) {
		return false
	}
	state := p.l.SaveState()
	defer p.l.RestoreState(state)
	next := p.l.NextToken()
	if p.peekTokenIs(lexer.CONST) {
		if next.Type != lexer.IDENT {
			return false
		}
		next = p.l.NextToken()
	}
	switch next.Type {
	case lexer.COMMA, lexer.ASSIGN:
		return true
	case lexer.EXTENDS:
		switch p.l.NextToken().Type {
		case lexer.ASSIGN, lexer.GT, lexer.SLASH:
			return false
		}
		return true
	}
	return false
}

// parseJSXElementExpression parses a JSX element or fragment, the current
// token being its '<'.
func (p *Parser) parseJSXElementExpression() Expression {
	lt := p.curToken
	p.jsxRewind()
	el := p.parseJSXElementRest(lt, p.jsxTagToken())
	p.jsxRefill()
	return el
}

// parseJSXElementRest parses an element after its '<', first being the token
// after it. It ends with the lexer just past the element.
func (p *Parser) parseJSXElementRest(lt, first *lexer.Token) *JSXElement {
	el := &JSXElement{Token: lt}
	tok := first
	if tok.Type == lexer.GT {
		p.parseJSXChildren(el)
		return el
	}
	el.TagName, tok = p.parseJSXTagName(tok)
	if el.TagName == nil {
		el.EndPos = tok.EndPos
		return el
	}
	if tok.Type == lexer.LT {
		p.jsxRefill()
		if args, err := p.parseTypeArguments(); err == nil {
			el.TypeArguments = args
		}
		p.jsxRewind()
		tok = p.jsxTagToken()
	}
	tok = p.parseJSXAttributes(el, tok)
	switch tok.Type {
	case lexer.SLASH:
		gt := p.jsxTagToken()
		if gt.Type != lexer.GT {
			p.addErrorWithCode(gt, "TS1005", "'>' expected.")
		}
		el.SelfClosing = true
		el.EndPos = gt.EndPos
	case lexer.GT:
		p.parseJSXChildren(el)
	default:
		p.addErrorWithCode(tok, "TS1005", "'>' expected.")
		el.EndPos = tok.StartPos
	}
	return el
}

// parseJSXTagName parses a tag name starting at tok: `name`, `a.b.c`,
// `ns:name` or `this`. It returns the name and the token after it.
func (p *Parser) parseJSXTagName(tok *lexer.Token) (Expression, *lexer.Token) {
	if tok.Type != lexer.IDENT {
		p.addErrorWithCode(tok, "TS1003", "Identifier expected.")
		return nil, tok
	}
	var name Expression
	if tok.Literal == "this" {
		name = &ThisExpression{Token: tok}
	} else {
		name = &Identifier{Token: tok, Value: tok.Literal}
	}
	next := p.jsxTagToken()
	if next.Type == lexer.COLON {
		local := p.jsxTagToken()
		if local.Type != lexer.IDENT {
			p.addErrorWithCode(local, "TS1003", "Identifier expected.")
			return name, local
		}
		return &JSXNamespacedName{Token: tok, Namespace: tok.Literal, Name: local.Literal}, p.jsxTagToken()
	}
	for next.Type == lexer.DOT {
		dot := next
		prop := p.jsxTagToken()
		if prop.Type != lexer.IDENT {
			p.addErrorWithCode(prop, "TS1003", "Identifier expected.")
			return name, prop
		}
		name = &MemberExpression{Token: dot, Object: name, Property: &Identifier{Token: prop, Value: prop.Literal}}
		next = p.jsxTagToken()
	}
	return name, next
}

// parseJSXAttributes parses attributes from tok up to the tag's '/' or '>',
// which it returns.
func (p *Parser) parseJSXAttributes(el *JSXElement, tok *lexer.Token) *lexer.Token {
	for {
		switch tok.Type {
		case lexer.SLASH, lexer.GT, lexer.EOF:
			return tok
		case lexer.LBRACE:
			attr := &JSXAttribute{Token: tok}
			p.jsxRefill()
			if !p.expectPeek(lexer.SPREAD) {
				p.jsxSkipToBrace()
			} else {
				p.nextToken()
				attr.Spread = p.parseExpression(COMMA)
				if !p.expectPeek(lexer.RBRACE) {
					p.jsxSkipToBrace()
				}
			}
			p.jsxRewind()
			if attr.Spread != nil {
				el.Attributes = append(el.Attributes, attr)
			}
			tok = p.jsxTagToken()
		case lexer.IDENT:
			attr := &JSXAttribute{Token: tok, Name: tok.Literal}
			tok = p.jsxTagToken()
			if tok.Type == lexer.COLON {
				local := p.jsxTagToken()
				attr.Name += ":" + local.Literal
				tok = p.jsxTagToken()
			}
			if tok.Type == lexer.ASSIGN {
				attr.Value, tok = p.parseJSXAttributeValue()
			}
			el.Attributes = append(el.Attributes, attr)
		default:
			p.addErrorWithCode(tok, "TS1145", "'{' or JSX element expected.")
			tok = p.jsxTagToken()
		}
	}
}

// parseJSXAttributeValue parses the value after an attribute's '=' and
// returns it with the token after it.
func (p *Parser) parseJSXAttributeValue() (Expression, *lexer.Token) {
	tok := p.jsxTagToken()
	switch tok.Type {
	case lexer.STRING:
		return &StringLiteral{Token: tok, Value: tok.Literal}, p.jsxTagToken()
	case lexer.LBRACE:
		container := &JSXExpressionContainer{Token: tok}
		p.jsxRefill()
		if p.peekTokenIs(lexer.RBRACE) {
			p.addErrorWithCode(tok, "TS17000", "JSX attributes must only be assigned a non-empty 'expression'.")
			p.nextToken()
		} else {
			p.nextToken()
			container.Expression = p.parseExpression(COMMA)
			if !p.expectPeek(lexer.RBRACE) {
				p.jsxSkipToBrace()
			}
		}
		p.jsxRewind()
		return container, p.jsxTagToken()
	case lexer.LT:
		el := p.parseJSXElementRest(tok, p.jsxTagToken())
		return el, p.jsxTagToken()
	}
	p.addErrorWithCode(tok, "TS1145", "'{' or JSX element expected.")
	return nil, tok
}

// jsxSkipToBrace recovers from a malformed `{...}` by skipping to its '}'.
func (p *Parser) jsxSkipToBrace() {
	for !p.curTokenIs(lexer.RBRACE) && !p.curTokenIs(lexer.EOF) {
		if p.peekTokenIs(lexer.EOF) {
			p.nextToken()
			return
		}
		p.nextToken()
	}
}

// parseJSXChildren parses the children of el after its opening tag's '>',
// through its closing tag.
func (p *Parser) parseJSXChildren(el *JSXElement) {
	for {
		tok := p.jsxConsume(p.l.ScanJSXChild())
		switch tok.Type {
		case lexer.JSX_TEXT:
			el.Children = append(el.Children, &JSXText{Token: tok, Value: tok.Literal})
		case lexer.LBRACE:
			container := &JSXExpressionContainer{Token: tok}
			p.jsxRefill()
			if p.peekTokenIs(lexer.RBRACE) {
				p.nextToken()
			} else {
				p.nextToken()
				if p.curTokenIs(lexer.SPREAD) {
					container.Spread = true
					p.nextToken()
				}
				container.Expression = p.parseExpression(LOWEST)
				if !p.expectPeek(lexer.RBRACE) {
					p.jsxSkipToBrace()
				}
			}
			p.jsxRewind()
			el.Children = append(el.Children, container)
		case lexer.LT:
			next := p.jsxTagToken()
			if next.Type == lexer.SLASH {
				p.parseJSXClosingTag(el, tok)
				return
			}
			el.Children = append(el.Children, p.parseJSXElementRest(tok, next))
		default: // EOF
			name := "<>"
			if el.TagName != nil {
				name = jsxTagNameText(el.TagName)
			}
			at := el.Token
			if el.TagName != nil {
				at = jsxNameToken(el.TagName)
			}
			p.addErrorWithCode(at, "TS17008", "JSX element '"+name+"' has no corresponding closing tag.")
			el.EndPos = tok.StartPos
			return
		}
	}
}

// parseJSXClosingTag parses `</name>` after its '/', lt being its '<'.
func (p *Parser) parseJSXClosingTag(el *JSXElement, lt *lexer.Token) {
	el.ClosingToken = lt
	tok := p.jsxTagToken()
	if tok.Type == lexer.GT {
		if el.TagName != nil {
			p.addErrorWithCode(tok, "TS17002", "Expected corresponding JSX closing tag for '"+jsxTagNameText(el.TagName)+"'.")
		}
		el.EndPos = tok.EndPos
		return
	}
	var name Expression
	name, tok = p.parseJSXTagName(tok)
	el.ClosingName = name
	if el.TagName == nil {
		if name != nil {
			p.addErrorWithCode(jsxNameToken(name), "TS17015", "Expected corresponding closing tag for JSX fragment.")
		}
	} else if name != nil && jsxTagNameText(name) != jsxTagNameText(el.TagName) {
		p.addErrorWithCode(jsxNameToken(name), "TS17002", "Expected corresponding JSX closing tag for '"+jsxTagNameText(el.TagName)+"'.")
	}
	if tok.Type != lexer.GT {
		p.addErrorWithCode(tok, "TS1005", "'>' expected.")
		el.EndPos = tok.StartPos
		return
	}
	el.EndPos = tok.EndPos
}

// jsxTagNameText is a tag name as written.
func jsxTagNameText(n Expression) string {
	switch n := n.(type) {
	case *Identifier:
		return n.Value
	case *ThisExpression:
		return "this"
	case *JSXNamespacedName:
		return n.Namespace + ":" + n.Name
	case *MemberExpression:
		if prop, ok := n.Property.(*Identifier); ok {
			return jsxTagNameText(n.Object) + "." + prop.Value
		}
	}
	return ""
}

// jsxNameToken is the first token of a tag name.
func jsxNameToken(n Expression) *lexer.Token {
	switch n := n.(type) {
	case *Identifier:
		return n.Token
	case *ThisExpression:
		return n.Token
	case *JSXNamespacedName:
		return n.Token
	case *MemberExpression:
		return jsxNameToken(n.Object)
	}
	return nil
}

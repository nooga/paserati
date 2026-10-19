package parser

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
)

// Error recovery in the style of tsc's parser. tsc never aborts a construct on a
// missing token or operand: it reports the error, fabricates a zero-width
// "missing" node and keeps parsing, so one mistake yields the one diagnostic tsc
// gives instead of a cascade. The primitives below give our token-pair parser
// (cur/peek) the same behaviour.

const maxRecoveriesPerToken = 3

// canRecover reports whether recovery is allowed: not while a speculative parse
// is running, whose failure the caller detects by return value.
func (p *Parser) canRecover() bool { return p.speculating == 0 }

// recoverBudget reports whether another recovery may happen at the given token
// position, and counts it. Recovery does not consume input, so a caller that
// retries in a loop could otherwise spin forever.
func (p *Parser) recoverBudget(pos int) bool {
	if p.stepBackPos != pos {
		p.stepBackPos = pos
		p.stepBackCount = 0
	}
	p.stepBackCount++
	return p.stepBackCount <= maxRecoveriesPerToken
}

// stepBackForMissing un-consumes the current token: afterwards cur is the token
// before it (the last token of the "missing" expression's left context) and peek
// is the offending token, as if an empty expression had been parsed between
// them. It reports false if that is not possible.
func (p *Parser) stepBackForMissing() bool {
	if p.prevToken == nil || p.l.HasPushedToken() || p.curTokenIs(lexer.EOF) {
		return false
	}
	if !p.recoverBudget(p.curToken.StartPos) {
		return false
	}
	p.l.PushBackToken(*p.peekToken)
	p.peekToken = p.curToken
	p.curToken = p.prevToken
	p.prevToken = nil
	return true
}

// missingExpression is the zero-width placeholder tsc calls a "missing
// identifier": it stands for an expression that was required but absent.
func (p *Parser) missingExpression() *Identifier {
	tok := &lexer.Token{Type: lexer.IDENT, Line: p.peekToken.Line, Column: p.peekToken.Column, StartPos: p.peekToken.StartPos, EndPos: p.peekToken.StartPos}
	return &Identifier{Token: tok, Value: ""}
}

// expectPeekRecover is expectPeek with tsc's parseExpected semantics: when the
// next token is not t it reports the error and carries on as if t had been
// present (a zero-width token becomes the current one) instead of failing the
// whole construct.
func (p *Parser) expectPeekRecover(t lexer.TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.peekError(t)
	if !p.canRecover() || !p.recoverBudget(p.peekToken.StartPos) {
		return false
	}
	p.prevToken = p.curToken
	p.curToken = &lexer.Token{Type: t, Literal: string(t), Line: p.curToken.Line, Column: p.curToken.Column, StartPos: p.peekToken.StartPos, EndPos: p.peekToken.StartPos}
	return true
}

// cannotStartStatement reports tokens that can begin neither a statement nor an
// expression (tsc: !isStartOfStatement()). Such a token at the start of a
// statement is skipped with TS1128.
func cannotStartStatement(t lexer.TokenType) bool {
	switch t {
	case lexer.RPAREN, lexer.RBRACKET, lexer.COMMA, lexer.COLON, lexer.ARROW, lexer.QUESTION,
		lexer.OPTIONAL_CHAINING, lexer.DOT, lexer.SPREAD,
		lexer.ASSIGN, lexer.PLUS_ASSIGN, lexer.MINUS_ASSIGN, lexer.ASTERISK_ASSIGN, lexer.SLASH_ASSIGN,
		lexer.REMAINDER_ASSIGN, lexer.EXPONENT_ASSIGN, lexer.BITWISE_AND_ASSIGN, lexer.BITWISE_OR_ASSIGN,
		lexer.BITWISE_XOR_ASSIGN, lexer.LEFT_SHIFT_ASSIGN, lexer.RIGHT_SHIFT_ASSIGN, lexer.UNSIGNED_RIGHT_SHIFT_ASSIGN,
		lexer.LOGICAL_AND_ASSIGN, lexer.LOGICAL_OR_ASSIGN, lexer.COALESCE_ASSIGN,
		lexer.ASTERISK, lexer.REMAINDER, lexer.EXPONENT, lexer.EQ, lexer.NOT_EQ, lexer.STRICT_EQ, lexer.STRICT_NOT_EQ,
		lexer.LE, lexer.GE, lexer.GT, lexer.RIGHT_SHIFT, lexer.UNSIGNED_RIGHT_SHIFT, lexer.LEFT_SHIFT,
		lexer.BITWISE_AND, lexer.BITWISE_XOR, lexer.PIPE, lexer.LOGICAL_AND, lexer.LOGICAL_OR, lexer.COALESCE,
		lexer.IN, lexer.INSTANCEOF, lexer.ELSE, lexer.CATCH, lexer.FINALLY, lexer.EXTENDS:
		return true
	}
	return false
}

// reportStatementStart reports TS1128 for a token that cannot start a statement.
func (p *Parser) reportStatementStart() {
	p.addErrorWithCode(p.curToken, errors.TS1128, "Declaration or statement expected.")
}

// startsExpression reports whether tok can begin an expression (tsc:
// isStartOfExpression), judged by whether the Pratt parser has a prefix rule.
func (p *Parser) startsExpression(tok *lexer.Token) bool {
	// A `/` or `/=` there is rescanned as a regular expression.
	if p.tscDiagnostics && statementOnlyKeyword(tok.Type) {
		return false
	}
	return p.prefixParseFns[tok.Type] != nil || tok.Type == lexer.SLASH || tok.Type == lexer.SLASH_ASSIGN
}

// statementOnlyKeyword reports reserved words that have a prefix parse
// function (as identifiers, for property-name positions) but can never start
// an expression in tsc.
func statementOnlyKeyword(t lexer.TokenType) bool {
	return t == lexer.RETURN || t == lexer.THROW
}

// parseArgumentsRecover parses call arguments with tsc's parseDelimitedList
// recovery. cur is the opening '(' on entry; on return cur is the closing token
// (real or synthesized). A missing comma is reported and the next argument parsed
// anyway; a token that cannot start an argument is reported (TS1135) and skipped.
func (p *Parser) parseArgumentsRecover(end lexer.TokenType) []Expression {
	list := []Expression{}
loop:
	for {
		switch {
		case p.startsExpression(p.peekToken):
			p.nextToken()
			if expr := p.parseExpression(ARG_SEPARATOR); expr != nil {
				list = append(list, expr)
			}
			switch {
			case p.peekTokenIs(lexer.COMMA):
				p.nextToken()
			case p.peekTokenIs(end), p.peekTokenIs(lexer.SEMICOLON), p.peekTokenIs(lexer.EOF):
				break loop
			default:
				p.addErrorWithCode(p.peekToken, errors.TS1005, "',' expected.")
			}
		case p.peekTokenIs(end), p.peekTokenIs(lexer.SEMICOLON), p.peekTokenIs(lexer.EOF),
			p.peekTokenIs(lexer.RBRACE), p.peekTokenIs(lexer.RBRACKET):
			break loop
		default:
			p.addErrorWithCode(p.peekToken, "TS1135", "Argument expression expected.")
			if p.tscDiagnostics && listAbortsAt(p.peekToken) && !p.peekTokenIs(lexer.ILLEGAL) {
				// abortParsingListOrMoveToNextToken: a statement start ends
				// the list. (tsc's scanner never yields an invalid
				// character as a token.)
				break loop
			}
			p.nextToken() // skip the offending token
		}
	}
	if !p.expectPeekRecover(end) {
		return nil
	}
	return list
}

// startsPropertyName reports whether tok can begin an object-literal member
// (tsc: isListElement(ObjectLiteralMembers)).
func (p *Parser) startsPropertyName(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.IDENT, lexer.STRING, lexer.NUMBER, lexer.BIGINT, lexer.LBRACKET, lexer.ASTERISK, lexer.SPREAD, lexer.PRIVATE_IDENT:
		return true
	}
	return p.isKeywordThatCanBeIdentifier(tok.Type)
}

// badDeclarationListStart handles a let/const/var keyword that is not followed by
// a binding. When the next token ends the statement (`;`, `}`, end of input or a
// line break) tsc parses an empty declaration list and its checker reports
// TS1123; otherwise the token is not a declaration (TS1134) and is skipped.
func (p *Parser) badDeclarationListStart(kw *lexer.Token, name string) Statement {
	if !p.canRecover() {
		p.addError(p.curToken, "expected identifier or destructuring pattern after '"+name+"', got "+string(p.curToken.Type))
		return nil
	}
	if p.curTokenIs(lexer.SEMICOLON) || p.curTokenIs(lexer.RBRACE) || p.curTokenIs(lexer.EOF) || p.curToken.Line > kw.Line || p.curTokenIs(lexer.IN) || p.curTokenIs(lexer.OF) {
		p.addErrorWithCode(&lexer.Token{Line: kw.Line, Column: kw.Column + len(name), StartPos: kw.EndPos, EndPos: kw.EndPos}, "TS1123", "Variable declaration list cannot be empty.")
		// Leave the terminator as the current token's predecessor so the caller's
		// statement loop sees it next.
		if p.stepBackForMissing() {
			return &ExpressionStatement{Token: kw, Expression: p.missingExpression()}
		}
		return nil
	}
	p.addErrorWithCode(p.curToken, "TS1134", "Variable declaration expected.")
	return nil
}

// consumeSignatureEnd reports whether a declaration without a body ends at the
// next token (tsc: parseFunctionBlockOrSemicolon): a ';' (consumed), or a
// token that lets a semicolon be inserted (end of line, '}' or end of input)
// when no '{' follows. It does not consume anything otherwise.
func (p *Parser) consumeSignatureEnd() bool {
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
		return true
	}
	if p.peekTokenIs(lexer.LBRACE) {
		return false
	}
	if p.peekToken.Line > p.curToken.Line || p.peekTokenIs(lexer.RBRACE) || p.peekTokenIs(lexer.EOF) {
		// Callers resume at the next member, as after a ';'-terminated signature.
		p.nextToken()
		return true
	}
	return false
}

// canFollowTypeArgumentsInExpression is tsc's rule for reading `a<b>` as type
// arguments rather than comparisons, when no '(' follows: the token after the
// '>' must not be '<', '>', '+' or '-', and must be on a new line, be a binary
// operator, or be unable to start an expression. cur is the closing '>'.
func (p *Parser) canFollowTypeArgumentsInExpression() bool {
	switch p.peekToken.Type {
	case lexer.LPAREN, lexer.TEMPLATE_START:
		return true
	case lexer.LT, lexer.GT, lexer.PLUS, lexer.MINUS:
		return false
	case lexer.LBRACE, lexer.IMPLEMENTS:
		if p.inHeritage > 0 {
			return true
		}
	}
	if p.peekToken.Line > p.curToken.Line || !p.startsExpression(p.peekToken) {
		return true
	}
	switch p.peekToken.Type {
	case lexer.LPAREN, lexer.LBRACKET, lexer.DOT, lexer.QUESTION, lexer.COMMA:
		return false
	}
	_, isBinary := precedences[p.peekToken.Type]
	return isBinary
}

// forHeadIsInOf looks ahead from the current token (the first token of a for-loop
// head expression) to the end of a balanced expression prefix and reports whether
// it is followed by `in` or `of` at depth 0, i.e. the head is a for-in/for-of
// left-hand side. Nothing is consumed.
func (p *Parser) forHeadIsInOf() bool {
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
	for i := 0; i < 400 && !p.curTokenIs(lexer.EOF); i++ {
		switch p.curToken.Type {
		case lexer.LPAREN, lexer.LBRACKET, lexer.LBRACE:
			depth++
		case lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE:
			depth--
			if depth < 0 {
				return false
			}
		case lexer.SEMICOLON:
			if depth == 0 {
				return false
			}
		case lexer.IN, lexer.OF:
			if depth == 0 {
				return true
			}
		case lexer.ASSIGN, lexer.COMMA:
			if depth == 0 {
				return false
			}
		}
		p.nextToken()
	}
	return false
}

// addGrammarErrorWithCode reports a diagnostic that tsc raises from a checker
// grammar check (suppressed when the file has parse diagnostics), whatever its code.
func (p *Parser) addGrammarErrorWithCode(tok *lexer.Token, code, msg string) {
	before := len(p.errors)
	p.addErrorWithCode(tok, code, msg)
	if len(p.errors) > before {
		if se, ok := p.errors[len(p.errors)-1].(*errors.SyntaxError); ok {
			se.Grammar = true
		}
	}
}

// addSemanticErrorWithCode reports a diagnostic tsc raises from the checker's
// plain error() (not a grammar check): it is neither suppressed by parse errors
// nor counted as one.
// hasSyntaxErrors reports whether parsing recorded an error other than a
// semantic one (which leaves the tree whole).
func (p *Parser) hasSyntaxErrors() bool {
	for _, e := range p.errors {
		if se, ok := e.(*errors.SyntaxError); !ok || !se.Semantic {
			return true
		}
	}
	return false
}

func (p *Parser) addSemanticErrorWithCode(tok *lexer.Token, code, msg string) {
	before := len(p.errors)
	p.addErrorWithCode(tok, code, msg)
	if len(p.errors) > before {
		if se, ok := p.errors[len(p.errors)-1].(*errors.SyntaxError); ok {
			se.Semantic = true
		}
	}
}

// startsDeclaration reports whether tok begins a declaration (used to tell a
// misplaced modifier from an identifier that happens to be named like one).
func (p *Parser) startsDeclaration(tok *lexer.Token) bool {
	switch tok.Type {
	case lexer.CLASS, lexer.FUNCTION, lexer.VAR, lexer.LET, lexer.CONST, lexer.ENUM, lexer.INTERFACE, lexer.ABSTRACT, lexer.ASYNC, lexer.TYPE:
		return true
	case lexer.IDENT:
		return tok.Literal == "namespace" || tok.Literal == "module" || tok.Literal == "declare"
	}
	return false
}

// isWordToken reports an identifier or keyword token (tsc's
// tokenIsIdentifierOrKeyword).
func isWordToken(t *lexer.Token) bool {
	switch t.Type {
	case lexer.IDENT:
		return true
	case lexer.STRING, lexer.NUMBER, lexer.BIGINT, lexer.TEMPLATE_START, lexer.EOF:
		return false
	}
	return t.Literal != "" && !isPunctuationToken(t)
}

// tscIsDeclaration is tsc's isDeclaration lookahead from the current token:
// whether the statement here, after any modifiers, is a declaration.
func (p *Parser) tscIsDeclaration() bool {
	toks := []lexer.Token{*p.curToken, *p.peekToken}
	if p.peekToken.Type != lexer.EOF {
		p.scanAhead(1, func(t lexer.Token) bool {
			toks = append(toks, t)
			return len(toks) < 16
		})
	}
	at := func(i int) *lexer.Token {
		if i < len(toks) {
			return &toks[i]
		}
		return &lexer.Token{Type: lexer.EOF}
	}
	sameLine := func(i int) bool { return i > 0 && at(i).Type != lexer.EOF && at(i).Line == at(i-1).Line }
	isIdent := func(t *lexer.Token) bool {
		return t.Type == lexer.IDENT || (isWordToken(t) && !tscReservedWords[t.Literal] && p.isKeywordThatCanBeIdentifier(t.Type))
	}
	for i := 0; i < len(toks); {
		t := at(i)
		switch t.Type {
		case lexer.VAR, lexer.LET, lexer.CONST, lexer.FUNCTION, lexer.CLASS, lexer.ENUM:
			return true
		case lexer.AWAIT:
			// Like `using`, an object pattern counts (reported later); an
			// array pattern could be element access.
			return sameLine(i+1) && at(i+1).Literal == "using" && sameLine(i+2) && (isIdent(at(i+2)) || at(i+2).Type == lexer.LBRACE)
		case lexer.INTERFACE, lexer.TYPE:
			return sameLine(i+1) && isIdent(at(i+1))
		case lexer.ABSTRACT, lexer.ASYNC, lexer.PRIVATE, lexer.PROTECTED, lexer.PUBLIC, lexer.READONLY:
			i++
			if !sameLine(i) {
				return false
			}
			continue
		case lexer.STATIC:
			i++
			continue
		case lexer.IMPORT:
			n := at(i + 1)
			return n.Type == lexer.STRING || n.Type == lexer.ASTERISK || n.Type == lexer.LBRACE || isWordToken(n)
		case lexer.EXPORT:
			i++
			if at(i).Type == lexer.TYPE {
				i++
			}
			switch at(i).Type {
			case lexer.ASSIGN, lexer.ASTERISK, lexer.LBRACE, lexer.DEFAULT, lexer.AS, lexer.AT:
				return true
			}
			continue
		case lexer.IDENT:
			switch t.Literal {
			case "using":
				return sameLine(i+1) && (isIdent(at(i+1)) || at(i+1).Type == lexer.LBRACE)
			case "module", "namespace":
				return sameLine(i+1) && (isIdent(at(i+1)) || at(i+1).Type == lexer.STRING)
			case "global":
				n := at(i + 1)
				return n.Type == lexer.LBRACE || n.Type == lexer.IDENT || n.Type == lexer.EXPORT
			case "accessor", "declare":
				i++
				if !sameLine(i) {
					return false
				}
				if t.Literal == "declare" && at(i).Type == lexer.TYPE {
					return true
				}
				continue
			}
		}
		return false
	}
	return false
}

// tscStatementModifier handles a class-member modifier (public, private,
// protected, static, readonly, accessor) at the start of a statement, as tsc's
// isStartOfStatement does: before a declaration it is a modifier its checker
// rejects (TS1044 at module level, TS1184 in a block); before another word on
// the same line it is no statement at all (TS1128), skipped, or inside a
// class body the end of every enclosing block. ok is false when it is just an
// identifier.
func (p *Parser) tscStatementModifier() (Statement, bool) {
	mod := p.curToken
	if p.tscIsDeclaration() {
		if mod.Type == lexer.IDENT {
			p.addGrammarErrorWithCode(mod, "TS1275", "'accessor' modifier can only appear on a property declaration.")
		} else if mod.Type != lexer.READONLY {
			if n := len(p.blockKinds); n == 0 || p.blockKinds[n-1] {
				p.addGrammarErrorWithCode(mod, "TS1044", "'"+mod.Literal+"' modifier cannot appear on a module or namespace element.")
			} else {
				p.addGrammarErrorWithCode(mod, "TS1184", "Modifiers cannot appear here.")
			}
		}
		for {
			switch p.curToken.Type {
			case lexer.PUBLIC, lexer.PRIVATE, lexer.PROTECTED, lexer.STATIC, lexer.READONLY:
				p.nextToken()
				continue
			case lexer.IDENT:
				if p.curToken.Literal == "accessor" {
					p.nextToken()
					continue
				}
			}
			break
		}
		return p.parseStatement(), true
	}
	if p.peekToken.Line != mod.Line || !isWordToken(p.peekToken) {
		return nil, false
	}
	p.reportStatementStart()
	if p.classBodyDepth > 0 && p.stepBackForMissing() {
		p.abortToClassMember = true
	}
	return nil, true
}

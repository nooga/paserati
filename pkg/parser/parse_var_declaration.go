package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/lexer"
)

// varDeclKind names the three variable-declaration keywords, so the single
// declarator-list parser below can serve all of them.
type varDeclKind int

const (
	varDeclLet varDeclKind = iota
	varDeclConst
	varDeclVar
)

func (k varDeclKind) keyword() string {
	switch k {
	case varDeclConst:
		return "const"
	case varDeclVar:
		return "var"
	default:
		return "let"
	}
}

// declListItem is one parsed declarator: either a plain binding
// (`x`, `x: T = e`) or a destructuring pattern (`{a} = e`, `[a] = e`), which is
// its own statement node.
type declListItem struct {
	declarator *VarDeclarator // set for a plain binding
	pattern    Statement      // set for a destructuring pattern
}

// parseVariableDeclarationList parses the declarator list of a let/const/var
// clause and returns the statement(s) it declares.
//
// Every declarator position accepts either an identifier or a binding pattern -
// ECMAScript's LexicalBinding/VariableDeclaration allows both in *any* position,
// not just the first (paserati#159). The three keyword-specific parsers used to
// each carry their own copy of this loop, and every copy only understood
// identifiers after a comma, so `let r = 1, {a} = obj` failed to parse while
// `let {a} = obj, r = 1` parsed as a *single* destructuring declaration whose
// initializer had swallowed `, r = 1` as a comma operator (paserati#160).
//
// Precondition: curToken is the first token of the first declarator (the
// keyword has already been consumed). On return curToken is the clause's last
// token, with an optional trailing semicolon consumed.
func (p *Parser) parseVariableDeclarationList(declToken *lexer.Token, kind varDeclKind) Statement {
	first, ok := p.parseDeclListItem(declToken, kind, kind.keyword())
	if !ok {
		return nil
	}

	items, ok := p.parseMoreDeclListItems([]declListItem{first}, declToken, kind)
	if !ok {
		return nil
	}

	// Optional semicolon terminating the whole clause. The destructuring
	// declaration parsers deliberately leave this to us so that a pattern in
	// the last declarator position doesn't consume the statement terminator
	// out from under this loop.
	if p.peekTokenIs(lexer.SEMICOLON) {
		p.nextToken()
	}

	return p.buildDeclarationStatements(declToken, kind, items)
}

// parseMoreDeclListItems parses the `, declarator` tail of a declarator list,
// appending to items. Precondition: curToken is the last token of the preceding
// declarator.
func (p *Parser) parseMoreDeclListItems(items []declListItem, declToken *lexer.Token, kind varDeclKind) ([]declListItem, bool) {
	for p.peekTokenIs(lexer.COMMA) {
		commaToken := p.peekToken
		p.nextToken() // Consume ','

		// TS1009: a trailing comma before end-of-statement or a statement keyword
		if p.peekTokenIs(lexer.RETURN) || p.peekTokenIs(lexer.BREAK) ||
			p.peekTokenIs(lexer.CONTINUE) || p.peekTokenIs(lexer.THROW) ||
			p.peekTokenIs(lexer.SEMICOLON) || p.peekTokenIs(lexer.EOF) ||
			p.peekTokenIs(lexer.RBRACE) {
			p.addError(commaToken, "Trailing comma not allowed.")
			break
		}

		p.nextToken() // Move to the first token of the next declarator

		item, ok := p.parseDeclListItem(declToken, kind, ",")
		if !ok {
			return nil, false
		}
		items = append(items, item)
	}
	return items, true
}

// parseDeclListItem parses a single declarator. Returns ok=false if the
// declarator could not be parsed (an error has been reported).
func (p *Parser) parseDeclListItem(declToken *lexer.Token, kind varDeclKind, precededBy string) (declListItem, bool) {
	switch {
	case p.curTokenIs(lexer.LBRACKET):
		decl := p.parseArrayDestructuringDeclaration(declToken, kind == varDeclConst, true)
		if decl == nil {
			return declListItem{}, false
		}
		return declListItem{pattern: decl}, true

	case p.curTokenIs(lexer.LBRACE):
		decl := p.parseObjectDestructuringDeclaration(declToken, kind == varDeclConst, true)
		if decl == nil {
			return declListItem{}, false
		}
		return declListItem{pattern: decl}, true

	case p.curTokenIsIdentLike():
		declarator := &VarDeclarator{}
		declarator.Name = &Identifier{Token: p.curToken, Value: p.curToken.Literal}

		if !p.parseDeclaratorAnnotation(declarator) {
			return declListItem{}, false
		}

		if kind == varDeclConst {
			// const requires an initializer.
			if !p.expectPeek(lexer.ASSIGN) {
				return declListItem{}, false
			}
			p.nextToken() // Move to the first token of the initializer
			declarator.Value = p.parseExpression(COMMA)
		} else if p.peekTokenIs(lexer.ASSIGN) {
			p.nextToken() // Consume '='
			p.nextToken() // Move to the first token of the initializer
			// COMMA precedence: an Initializer is an AssignmentExpression, so it
			// must never swallow the comma separating declarators.
			declarator.Value = p.parseExpression(COMMA)
		}
		return declListItem{declarator: declarator}, true

	default:
		p.addError(p.curToken, fmt.Sprintf(
			"expected identifier or destructuring pattern after '%s', got %s",
			precededBy, p.curToken.Type))
		return declListItem{}, false
	}
}

// buildDeclarationStatements turns the parsed declarators into AST statements,
// preserving the pre-existing single-statement shapes wherever it can: an
// all-identifier list stays one Let/Const/VarStatement, and a lone destructuring
// declarator stays a bare Array/ObjectDestructuringDeclaration. Only a genuinely
// mixed (or multi-pattern) list needs a DeclarationGroup.
func (p *Parser) buildDeclarationStatements(declToken *lexer.Token, kind varDeclKind, items []declListItem) Statement {
	if len(items) == 0 {
		return nil
	}

	patterns := 0
	for _, it := range items {
		if it.pattern != nil {
			patterns++
		}
	}

	if patterns == 0 {
		declarators := make([]*VarDeclarator, 0, len(items))
		for _, it := range items {
			declarators = append(declarators, it.declarator)
		}
		return newVarLikeStatement(declToken, kind, declarators)
	}

	if len(items) == 1 {
		return items[0].pattern
	}

	group := &DeclarationGroup{Token: declToken}
	for _, it := range items {
		if it.pattern != nil {
			group.Declarations = append(group.Declarations, it.pattern)
			continue
		}
		// One statement per plain binding: the compiler's Let/Const/Var paths
		// alias the statement's legacy Name/Value fields to the declarator being
		// compiled, so a single declarator per statement keeps those consistent.
		group.Declarations = append(group.Declarations,
			newVarLikeStatement(declToken, kind, []*VarDeclarator{it.declarator}))
	}
	return group
}

// newVarLikeStatement builds the Let/Const/VarStatement for a declarator list,
// including the legacy first-declarator fields the checker and compiler still
// read.
func newVarLikeStatement(declToken *lexer.Token, kind varDeclKind, declarators []*VarDeclarator) Statement {
	first := declarators[0]
	switch kind {
	case varDeclConst:
		return &ConstStatement{
			Token:          declToken,
			Declarations:   declarators,
			Name:           first.Name,
			TypeAnnotation: first.TypeAnnotation,
			Value:          first.Value,
			ComputedType:   first.ComputedType,
		}
	case varDeclVar:
		return &VarStatement{
			Token:          declToken,
			Declarations:   declarators,
			Name:           first.Name,
			TypeAnnotation: first.TypeAnnotation,
			Value:          first.Value,
			ComputedType:   first.ComputedType,
		}
	default:
		return &LetStatement{
			Token:          declToken,
			Declarations:   declarators,
			Name:           first.Name,
			TypeAnnotation: first.TypeAnnotation,
			Value:          first.Value,
			ComputedType:   first.ComputedType,
		}
	}
}

// forInitDeclKind reports the declaration keyword and keyword token of an
// already-parsed for-loop head binding, and whether it is a variable
// declaration at all (a for initializer can also be a plain expression).
func forInitDeclKind(stmt Statement) (varDeclKind, *lexer.Token, bool) {
	kindOf := func(isConst bool, tok *lexer.Token) varDeclKind {
		if isConst {
			return varDeclConst
		}
		if tok != nil && tok.Literal == "var" {
			return varDeclVar
		}
		return varDeclLet
	}
	// A failed sub-parse can leave a typed-nil pointer in the interface, which
	// is not == nil; treat that as "not a declaration" rather than dereferencing.
	switch s := stmt.(type) {
	case *LetStatement:
		if s == nil {
			return varDeclLet, nil, false
		}
		return varDeclLet, s.Token, true
	case *ConstStatement:
		if s == nil {
			return varDeclLet, nil, false
		}
		return varDeclConst, s.Token, true
	case *VarStatement:
		if s == nil {
			return varDeclLet, nil, false
		}
		return varDeclVar, s.Token, true
	case *ArrayDestructuringDeclaration:
		if s == nil {
			return varDeclLet, nil, false
		}
		return kindOf(s.IsConst, s.Token), s.Token, true
	case *ObjectDestructuringDeclaration:
		if s == nil {
			return varDeclLet, nil, false
		}
		return kindOf(s.IsConst, s.Token), s.Token, true
	}
	return varDeclLet, nil, false
}

// continueForInitDeclarators parses the `, declarator` tail of a `for` loop
// initializer, given the already-parsed first declaration. Like statement
// position, any declarator here may be a binding pattern -
// `for (let i = 0, {a} = obj; ...)` is legal (paserati#159).
//
// Returns the statement to use as the loop's initializer: `first` itself,
// extended in place when every extra declarator is a plain binding (which
// preserves flags such as LetStatement.IsUsing), or a DeclarationGroup when a
// pattern is involved.
func (p *Parser) continueForInitDeclarators(first Statement, declToken *lexer.Token, kind varDeclKind) Statement {
	if !p.peekTokenIs(lexer.COMMA) {
		return first
	}

	extra, ok := p.parseMoreDeclListItems(nil, declToken, kind)
	if !ok {
		return nil
	}
	if len(extra) == 0 {
		return first
	}

	patterns := 0
	for _, it := range extra {
		if it.pattern != nil {
			patterns++
		}
	}

	if patterns == 0 {
		// All plain bindings: append them to the existing declaration so its
		// identity and flags survive.
		declarators := make([]*VarDeclarator, 0, len(extra))
		for _, it := range extra {
			declarators = append(declarators, it.declarator)
		}
		switch s := first.(type) {
		case *LetStatement:
			s.Declarations = append(s.Declarations, declarators...)
			return s
		case *ConstStatement:
			s.Declarations = append(s.Declarations, declarators...)
			return s
		case *VarStatement:
			s.Declarations = append(s.Declarations, declarators...)
			return s
		}
	}

	group := &DeclarationGroup{Token: declToken, Declarations: []Statement{first}}
	for _, it := range extra {
		if it.pattern != nil {
			group.Declarations = append(group.Declarations, it.pattern)
			continue
		}
		group.Declarations = append(group.Declarations,
			newVarLikeStatement(declToken, kind, []*VarDeclarator{it.declarator}))
	}
	return group
}

// appendFlatteningGroups appends stmt to a statement list, splicing a
// DeclarationGroup's members in as siblings. A group introduces no scope, and
// the hoisting/pre-registration passes in the checker and compiler walk
// statement lists looking for declaration statements by type - so wherever the
// grammar allows a statement list, the members belong directly in it.
func appendFlatteningGroups(list []Statement, stmt Statement) []Statement {
	if group, ok := stmt.(*DeclarationGroup); ok {
		return append(list, group.Declarations...)
	}
	return append(list, stmt)
}

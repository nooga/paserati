package parser

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"
)

// Declaration-scope early errors (ECMA-262 static semantics).
//
// The rules checked here all compare two name sets of one scope:
// LexicallyDeclaredNames (let/const/class, and function declarations when the
// scope is a Block or CaseBlock) and VarDeclaredNames (every `var` in the
// scope's statements, however deeply nested in blocks and loops, but not in
// nested functions; plus the top-level function declarations of a function
// body or script). A lexical name may appear once and never alongside a var
// name. The checks run as each scope finishes parsing, so they are true early
// errors: nothing in the program executes.
//
// TypeScript-only declarations (declare, overload signatures, interfaces,
// enums, namespaces, type aliases) are ignored - they either merge by design
// or are checked by the type checker.

// scopedName is one binding a declaration introduces.
type scopedName struct {
	name string
	tok  *lexer.Token
	// plainFunc marks a FunctionDeclaration that is neither async nor a
	// generator: Annex B.3.2.4 lets sloppy-mode blocks repeat those.
	plainFunc bool
	// forOfVar marks a `var` bound by a for-of head, which Annex B.3.4 still
	// forbids from redeclaring a catch parameter.
	forOfVar bool
}

func (p *Parser) redeclarationError(n scopedName) {
	p.addError(n.tok, fmt.Sprintf("SyntaxError: Identifier '%s' has already been declared", n.name))
}

// isStrictContext reports whether the code being parsed is strict: a "use
// strict" directive is in effect, or we are inside a class body.
func (p *Parser) isStrictContext() bool {
	return p.strictMode || p.classBodyDepth > 0
}

// forEachBindingIdentifier calls fn for every binding identifier in a binding
// target, in source order and without de-duplication (duplicates are exactly
// what the callers look for).
func forEachBindingIdentifier(target Expression, fn func(*Identifier)) {
	switch t := target.(type) {
	case nil:
	case *Identifier:
		fn(t)
	case *AssignmentExpression:
		forEachBindingIdentifier(t.Left, fn)
	case *SpreadElement:
		forEachBindingIdentifier(t.Argument, fn)
	case *ObjectLiteral:
		for _, prop := range t.Properties {
			if prop == nil {
				continue
			}
			if prop.Value != nil {
				forEachBindingIdentifier(prop.Value, fn)
			} else {
				forEachBindingIdentifier(prop.Key, fn)
			}
		}
	case *ArrayLiteral:
		for _, elem := range t.Elements {
			forEachBindingIdentifier(elem, fn)
		}
	case *ObjectParameterPattern:
		forEachObjectPatternIdentifier(t.Properties, t.RestProperty, fn)
	case *ArrayParameterPattern:
		forEachArrayPatternIdentifier(t.Elements, fn)
	}
}

func forEachObjectPatternIdentifier(props []*DestructuringProperty, rest *DestructuringElement, fn func(*Identifier)) {
	for _, prop := range props {
		if prop != nil {
			forEachBindingIdentifier(prop.Target, fn)
		}
	}
	if rest != nil {
		forEachBindingIdentifier(rest.Target, fn)
	}
}

func forEachArrayPatternIdentifier(elems []*DestructuringElement, fn func(*Identifier)) {
	for _, elem := range elems {
		if elem != nil {
			forEachBindingIdentifier(elem.Target, fn)
		}
	}
}

// declarationKind classifies a declaration statement's keyword.
type declarationKind int

const (
	notADeclaration declarationKind = iota
	varDeclaration
	lexicalDeclaration
)

// declarationBindings reports the kind of a let/const/var/using declaration
// statement and calls fn for each name it binds. TypeScript `declare` forms
// report notADeclaration.
func declarationBindings(stmt Statement, fn func(*Identifier)) declarationKind {
	kindOf := func(t *lexer.Token) declarationKind {
		if t != nil && t.Type == lexer.VAR {
			return varDeclaration
		}
		return lexicalDeclaration
	}
	declarators := func(ds []*VarDeclarator, legacy *Identifier) {
		if len(ds) == 0 {
			if legacy != nil {
				fn(legacy)
			}
			return
		}
		for _, d := range ds {
			if d != nil && d.Name != nil {
				fn(d.Name)
			}
		}
	}
	switch s := stmt.(type) {
	case *LetStatement:
		if s == nil || s.Declare {
			return notADeclaration
		}
		declarators(s.Declarations, s.Name)
		return lexicalDeclaration
	case *ConstStatement:
		if s == nil || s.Declare {
			return notADeclaration
		}
		declarators(s.Declarations, s.Name)
		return lexicalDeclaration
	case *VarStatement:
		if s == nil || s.Declare {
			return notADeclaration
		}
		declarators(s.Declarations, s.Name)
		return varDeclaration
	case *ArrayDestructuringDeclaration:
		if s == nil {
			return notADeclaration
		}
		forEachArrayPatternIdentifier(s.Elements, fn)
		return kindOf(s.Token)
	case *ObjectDestructuringDeclaration:
		if s == nil {
			return notADeclaration
		}
		forEachObjectPatternIdentifier(s.Properties, s.RestProperty, fn)
		return kindOf(s.Token)
	case *DeclarationGroup:
		if s == nil {
			return notADeclaration
		}
		kind := notADeclaration
		for _, d := range s.Declarations {
			if k := declarationBindings(d, fn); k != notADeclaration {
				kind = k
			}
		}
		return kind
	}
	return notADeclaration
}

// functionDeclarationOf returns the FunctionLiteral a statement declares, or
// nil when the statement is not a function declaration. A declaration is an
// unparenthesized named function literal in statement position; a bodiless
// literal is a TypeScript overload signature and declares nothing.
func functionDeclarationOf(stmt Statement) *FunctionLiteral {
	es, ok := stmt.(*ExpressionStatement)
	if !ok || es == nil {
		return nil
	}
	fl, ok := es.Expression.(*FunctionLiteral)
	if !ok || fl == nil || fl.Parenthesized || fl.Name == nil || fl.Body == nil {
		return nil
	}
	return fl
}

// labelledFunctionOf returns the function declaration at the end of a chain
// of labels (`a: b: function f() {}`), or nil.
func labelledFunctionOf(stmt Statement) *FunctionLiteral {
	for {
		ls, ok := stmt.(*LabeledStatement)
		if !ok || ls == nil {
			return functionDeclarationOf(stmt)
		}
		stmt = ls.Statement
	}
}

// statementListNames computes the LexicallyDeclaredNames and the
// VarDeclaredNames of a statement list. topLevel selects the function-body /
// script flavour, where function declarations are var-scoped.
func statementListNames(stmts []Statement, topLevel bool) (lex, vars []scopedName) {
	for _, stmt := range stmts {
		lex, vars = statementNames(stmt, topLevel, lex, vars)
	}
	return lex, vars
}

func statementNames(stmt Statement, topLevel bool, lex, vars []scopedName) ([]scopedName, []scopedName) {
	if export, ok := stmt.(*ExportNamedDeclaration); ok && export != nil {
		if export.Declaration == nil {
			return lex, vars
		}
		stmt = export.Declaration
	}
	if fl := labelledFunctionOf(stmt); fl != nil {
		n := scopedName{name: fl.Name.Value, tok: fl.Name.Token, plainFunc: !fl.IsAsync && !fl.IsGenerator}
		if topLevel {
			vars = append(vars, n)
		} else {
			lex = append(lex, n)
		}
		return lex, vars
	}
	if cd, ok := stmt.(*ClassDeclaration); ok {
		if cd != nil && !cd.Declare && cd.Name != nil {
			lex = append(lex, scopedName{name: cd.Name.Value, tok: cd.Name.Token})
		}
		return lex, vars
	}
	var names []scopedName
	kind := declarationBindings(stmt, func(id *Identifier) {
		names = append(names, scopedName{name: id.Value, tok: id.Token})
	})
	switch kind {
	case lexicalDeclaration:
		return append(lex, names...), vars
	case varDeclaration:
		return lex, append(vars, names...)
	}
	return lex, collectVarNames(stmt, vars)
}

// collectVarNames appends the VarDeclaredNames of stmt: its `var` bindings,
// looking through nested statements but not into functions or classes.
func collectVarNames(stmt Statement, vars []scopedName) []scopedName {
	addVars := func(s Statement, forOf bool) {
		declarationBindingsIfVar(s, func(id *Identifier) {
			vars = append(vars, scopedName{name: id.Value, tok: id.Token, forOfVar: forOf})
		})
	}
	block := func(b *BlockStatement) {
		if b != nil {
			for _, s := range b.Statements {
				vars = collectVarNames(s, vars)
			}
		}
	}
	switch s := stmt.(type) {
	case *VarStatement, *ArrayDestructuringDeclaration, *ObjectDestructuringDeclaration, *DeclarationGroup:
		addVars(s, false)
	case *ExportNamedDeclaration:
		if s != nil && s.Declaration != nil {
			addVars(s.Declaration, false)
		}
	case *BlockStatement:
		block(s)
	case *IfStatement:
		if s != nil {
			block(s.Consequence)
			block(s.Alternative)
		}
	case *WhileStatement:
		if s != nil {
			block(s.Body)
		}
	case *DoWhileStatement:
		if s != nil {
			block(s.Body)
		}
	case *ForStatement:
		if s != nil {
			addVars(s.Initializer, false)
			block(s.Body)
		}
	case *ForInStatement:
		if s != nil {
			addVars(s.Variable, false)
			block(s.Body)
		}
	case *ForOfStatement:
		if s != nil {
			addVars(s.Variable, true)
			block(s.Body)
		}
	case *LabeledStatement:
		if s != nil {
			vars = collectVarNames(s.Statement, vars)
		}
	case *WithStatement:
		if s != nil {
			vars = collectVarNames(s.Body, vars)
		}
	case *TryStatement:
		if s != nil {
			block(s.Body)
			if s.CatchClause != nil {
				block(s.CatchClause.Body)
			}
			block(s.FinallyBlock)
		}
	case *SwitchStatement:
		if s != nil {
			for _, c := range s.Cases {
				if c != nil {
					block(c.Body)
				}
			}
		}
	}
	return vars
}

func declarationBindingsIfVar(stmt Statement, fn func(*Identifier)) {
	if stmt == nil {
		return
	}
	var names []*Identifier
	if declarationBindings(stmt, func(id *Identifier) { names = append(names, id) }) == varDeclaration {
		for _, id := range names {
			fn(id)
		}
	}
}

// checkLexicalNames reports duplicate lexical names and lexical names that
// are also var names. allowDuplicateFunctions applies Annex B.3.2.4: sloppy
// blocks may repeat plain function declarations.
func (p *Parser) checkLexicalNames(lex, vars []scopedName, allowDuplicateFunctions bool) {
	if len(lex) == 0 {
		return
	}
	seen := make(map[string]scopedName, len(lex))
	for _, n := range lex {
		if prev, dup := seen[n.name]; dup {
			if !(allowDuplicateFunctions && prev.plainFunc && n.plainFunc) {
				p.redeclarationError(n)
			}
			continue
		}
		seen[n.name] = n
	}
	for _, v := range vars {
		if _, clash := seen[v.name]; clash {
			p.redeclarationError(v)
		}
	}
}

// checkBlockScope applies the Block / CaseBlock early errors to a statement
// list.
func (p *Parser) checkBlockScope(stmts []Statement) {
	lex, vars := statementListNames(stmts, false)
	p.checkLexicalNames(lex, vars, !p.isStrictContext())
}

// checkSwitchScope applies the CaseBlock early errors: all clauses share one
// scope.
func (p *Parser) checkSwitchScope(stmt *SwitchStatement) {
	if stmt == nil {
		return
	}
	var lex, vars []scopedName
	for _, c := range stmt.Cases {
		if c == nil || c.Body == nil {
			continue
		}
		l, v := statementListNames(c.Body.Statements, false)
		lex = append(lex, l...)
		vars = append(vars, v...)
	}
	p.checkLexicalNames(lex, vars, !p.isStrictContext())
}

// checkScriptScope applies the Script early errors to the top level.
func (p *Parser) checkScriptScope(stmts []Statement) {
	lex, vars := statementListNames(stmts, true)
	p.checkLexicalNames(lex, vars, false)
}

// parameterNames lists the bound names of a formal parameter list and
// reports whether the list is simple (plain identifiers only).
func parameterNames(params []*Parameter, rest *RestParameter) (names []scopedName, simple bool) {
	simple = rest == nil
	add := func(id *Identifier) { names = append(names, scopedName{name: id.Value, tok: id.Token}) }
	for _, param := range params {
		if param == nil || param.IsThis {
			continue
		}
		if param.DefaultValue != nil || param.Pattern != nil {
			simple = false
		}
		if param.Pattern != nil {
			forEachBindingIdentifier(param.Pattern, add)
		} else if param.Name != nil {
			add(param.Name)
		}
	}
	if rest != nil {
		if rest.Pattern != nil {
			forEachBindingIdentifier(rest.Pattern, add)
		} else if rest.Name != nil {
			add(rest.Name)
		}
	}
	return names, simple
}

// hasUseStrictDirective reports whether a body's directive prologue contains
// "use strict".
func hasUseStrictDirective(body *BlockStatement) bool {
	if body == nil {
		return false
	}
	for _, stmt := range body.Statements {
		es, ok := stmt.(*ExpressionStatement)
		if !ok || es == nil {
			return false
		}
		lit, ok := es.Expression.(*StringLiteral)
		if !ok || lit == nil {
			return false
		}
		if lit.Value == "use strict" && !lit.HasEscape {
			return true
		}
	}
	return false
}

// checkFunctionScope applies the FunctionBody scope early errors: the body's
// lexical names against its var names and parameter names. Parameter-list
// rules (duplicates, "use strict" with non-simple parameters) live in
// early_errors.go, which knows each function's final strictness.
func (p *Parser) checkFunctionScope(params []*Parameter, rest *RestParameter, body *BlockStatement) {
	if body == nil {
		return
	}
	paramNames, _ := parameterNames(params, rest)
	lex, vars := statementListNames(body.Statements, true)
	p.checkLexicalNames(lex, vars, false)
	if len(lex) > 0 && len(paramNames) > 0 {
		params := make(map[string]bool, len(paramNames))
		for _, n := range paramNames {
			params[n.name] = true
		}
		for _, n := range lex {
			if params[n.name] {
				p.redeclarationError(n)
			}
		}
	}
}

// checkCatchScope applies the Catch early errors: duplicate parameter names,
// and parameter names redeclared in the catch block. Annex B.3.4 permits a
// `var` of the same name as a plain-identifier parameter, except one bound by
// a for-of head.
func (p *Parser) checkCatchScope(clause *CatchClause) {
	if clause == nil || clause.Parameter == nil || clause.Body == nil {
		return
	}
	var paramNames []scopedName
	forEachBindingIdentifier(clause.Parameter, func(id *Identifier) {
		paramNames = append(paramNames, scopedName{name: id.Value, tok: id.Token})
	})
	params := make(map[string]bool, len(paramNames))
	for _, n := range paramNames {
		if params[n.name] {
			p.redeclarationError(n)
		}
		params[n.name] = true
	}
	_, simpleParam := clause.Parameter.(*Identifier)
	lex, vars := statementListNames(clause.Body.Statements, false)
	for _, n := range lex {
		if params[n.name] {
			p.redeclarationError(n)
		}
	}
	for _, v := range vars {
		if params[v.name] && (!simpleParam || v.forOfVar) {
			p.redeclarationError(v)
		}
	}
}

// checkForScope applies the early errors of a for / for-in / for-of head that
// declares let/const/using bindings: no binding named `let`, no duplicates,
// and none redeclared by a var in the loop body.
func (p *Parser) checkForScope(stmt Statement) {
	var head Statement
	var body *BlockStatement
	switch s := stmt.(type) {
	case *ForStatement:
		if s == nil {
			return
		}
		head, body = s.Initializer, s.Body
	case *ForInStatement:
		if s == nil {
			return
		}
		head, body = s.Variable, s.Body
	case *ForOfStatement:
		if s == nil {
			return
		}
		head, body = s.Variable, s.Body
	default:
		return
	}
	if head == nil {
		return
	}
	var names []scopedName
	if declarationBindings(head, func(id *Identifier) {
		names = append(names, scopedName{name: id.Value, tok: id.Token})
	}) != lexicalDeclaration {
		return
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if n.name == "let" {
			p.addError(n.tok, "SyntaxError: let is disallowed as a lexically bound name")
		}
		if seen[n.name] {
			p.redeclarationError(n)
		}
		seen[n.name] = true
	}
	if body == nil {
		return
	}
	var vars []scopedName
	for _, s := range body.Statements {
		vars = collectVarNames(s, vars)
	}
	for _, v := range vars {
		if seen[v.name] {
			p.redeclarationError(v)
		}
	}
}

// checkLexicalDeclarationNames rejects `let` as a name bound by a let/const
// declaration (13.3.1.1).
func (p *Parser) checkLexicalDeclarationNames(stmt Statement) {
	var bad *Identifier
	if declarationBindings(stmt, func(id *Identifier) {
		if bad == nil && id.Value == "let" {
			bad = id
		}
	}) == lexicalDeclaration && bad != nil {
		p.addError(bad.Token, "SyntaxError: let is disallowed as a lexically bound name")
	}
}

// subStatementContext names the construct whose Statement child is being
// parsed, for the statement-position early errors.
type subStatementContext int

const (
	subStatementIf    subStatementContext = iota // if / else
	subStatementOther                            // loops and with
	subStatementLabel                            // a LabelledItem
)

// labelEntry is one label enclosing the statement being parsed.
type labelEntry struct {
	name string
	// loop: the label (directly or through further labels) labels an
	// iteration statement, so `continue name` may target it.
	loop bool
	// chained: the labelled item is itself a labelled statement.
	chained bool
}

// pushLabel enters a label for the labelled item starting at curToken,
// reporting a duplicate of an enclosing label and `yield` as a label in
// strict code.
func (p *Parser) pushLabel(tok *lexer.Token, name string) {
	if p.isStrictContext() && name == "yield" {
		p.addError(tok, "SyntaxError: Unexpected strict mode reserved word 'yield'")
	}
	for _, outer := range p.labels {
		if outer.name == name {
			p.addError(tok, fmt.Sprintf("SyntaxError: Label '%s' has already been declared", name))
			break
		}
	}
	entry := labelEntry{name: name}
	switch {
	case p.curTokenIs(lexer.FOR), p.curTokenIs(lexer.WHILE), p.curTokenIs(lexer.DO):
		entry.loop = true
		// `a: b: while (...)` - every label of the chain labels the loop.
		for i := len(p.labels) - 1; i >= 0 && p.labels[i].chained; i-- {
			p.labels[i].loop = true
		}
	case p.peekTokenIs(lexer.COLON):
		entry.chained = true
	}
	p.labels = append(p.labels, entry)
}

// checkContinueLabel reports `continue L` where L is not the label of an
// enclosing iteration statement in the current function.
func (p *Parser) checkContinueLabel(label *Identifier) {
	for i := len(p.labels) - 1; i >= 0; i-- {
		if p.labels[i].name == label.Value {
			if !p.labels[i].loop {
				p.addError(label.Token, fmt.Sprintf("SyntaxError: Illegal continue statement: '%s' does not denote an iteration statement", label.Value))
			}
			return
		}
	}
	p.addError(label.Token, fmt.Sprintf("SyntaxError: Undefined label '%s'", label.Value))
}

// parseSubStatement parses the Statement child of if/else, a loop, with, or a
// label, with curToken at its first token. Such positions take a Statement,
// not a Declaration: let/const/class and generator/async function
// declarations are errors, and so are plain function declarations except
// where Annex B allows them in sloppy code (B.3.3 for if, B.3.2 for labels).
// A labelled function is never allowed as the body of if, a loop, or with.
func (p *Parser) parseSubStatement(ctx subStatementContext) Statement {
	startTok := p.curToken
	// ExpressionStatement's lookahead restriction: `let [` can only start a
	// declaration, even when a line break separates the two tokens.
	letBracket := p.curTokenIs(lexer.LET) && p.peekTokenIs(lexer.LBRACKET)
	p.inSubStatement = true
	stmt := p.parseStatement()
	if stmt == nil {
		return nil
	}
	strict := p.isStrictContext()
	if letBracket {
		p.addError(startTok, "SyntaxError: Lexical declaration cannot appear in a single-statement context")
		return stmt
	}
	if cd, ok := stmt.(*ClassDeclaration); ok && cd != nil {
		p.addError(startTok, "SyntaxError: Class declaration cannot appear in a single-statement context")
		return stmt
	}
	if declarationBindings(stmt, func(*Identifier) {}) == lexicalDeclaration {
		p.addError(startTok, "SyntaxError: Lexical declaration cannot appear in a single-statement context")
		return stmt
	}
	if fl := functionDeclarationOf(stmt); fl != nil {
		switch {
		case fl.IsAsync || fl.IsGenerator:
			p.addError(startTok, "SyntaxError: Async functions and generators can only be declared at the top level or inside a block")
		case strict:
			p.addError(startTok, "SyntaxError: In strict mode code, functions can only be declared at top level or inside a block")
		case ctx != subStatementIf && ctx != subStatementLabel:
			p.addError(startTok, "SyntaxError: Function declarations are not allowed in this statement position")
		}
		return stmt
	}
	if ctx != subStatementLabel {
		if fl := labelledFunctionOf(stmt); fl != nil {
			p.addError(startTok, "SyntaxError: A labelled function declaration cannot be the body of this statement")
		}
	}
	return stmt
}

// checkDeclarationEnd reports a let/const/var statement that is neither
// terminated by `;` nor followed by a point where ASI applies (a line break,
// `}`, or the end of input): `let x 0` is not two statements.
func (p *Parser) checkDeclarationEnd(stmt Statement) {
	if stmt == nil || p.curTokenIs(lexer.SEMICOLON) {
		return
	}
	if p.peekTokenIs(lexer.RBRACE) || p.peekTokenIs(lexer.EOF) || p.peekToken.Line > p.curToken.Line {
		return
	}
	p.addErrorWithCode(p.peekToken, errors.TS1005, "';' expected.")
}

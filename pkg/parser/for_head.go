package parser

import "github.com/nooga/paserati/pkg/lexer"

// checkForInOfHead reports a for-in/of declaration head with an initializer
// or more than one binding. Annex B.3.5 keeps `for (var x = e in o)` legal in
// sloppy code, for a single plain var binding only.
func (p *Parser) checkForInOfHead(head Statement, isOf bool) {
	kind := "for-in"
	if isOf {
		kind = "for-of"
	}
	bad := func(tok *lexer.Token) {
		if isOf {
			p.addGrammarErrorWithCode(tok, "TS1190", "The variable declaration of a 'for...of' statement cannot have an initializer.")
		} else {
			p.addGrammarErrorWithCode(tok, "TS1189", "The variable declaration of a 'for...in' statement cannot have an initializer.")
		}
	}
	declarators := func(tok *lexer.Token, decls []*VarDeclarator, isVar bool) {
		if len(decls) > 1 {
			if isOf {
				p.addGrammarErrorWithCode(tok, "TS1188", "Only a single variable declaration is allowed in a 'for...of' statement.")
			} else {
				p.addGrammarErrorWithCode(tok, "TS1091", "Only a single variable declaration is allowed in a 'for...in' statement.")
			}
			return
		}
		if len(decls) == 1 && decls[0].Value != nil && (isOf || !isVar || p.strictMode) {
			bad(tok)
		}
	}
	switch s := head.(type) {
	case *ExpressionStatement:
		// `for (f() of x)`: a direct call is an invalid assignment target (web-compat
		// ReferenceError at run time only in sloppy code).
		if _, isCall := s.Expression.(*CallExpression); isCall && p.strictMode {
			p.addError(s.Token, "Invalid left-hand side in "+kind+" statement")
		}
	case *VarStatement:
		declarators(s.Token, s.Declarations, true)
	case *LetStatement:
		if !isOf && (s.IsUsing || s.IsAwaitUsing) {
			p.addError(s.Token, "'using' declarations are not allowed in a for-in statement")
			return
		}
		declarators(s.Token, s.Declarations, false)
	case *ConstStatement:
		declarators(s.Token, s.Declarations, false)
	case *ArrayDestructuringDeclaration:
		if s.Value != nil {
			bad(s.Token)
		}
	case *ObjectDestructuringDeclaration:
		if s.Value != nil {
			bad(s.Token)
		}
	}
}

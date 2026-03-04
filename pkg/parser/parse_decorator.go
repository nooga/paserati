package parser

import (
	"github.com/nooga/paserati/pkg/lexer"
)

// parseDecoratedStatement parses a decorated class declaration.
// Called when curToken is '@'. After parsing decorators, expects 'class', 'abstract class',
// or 'export [default] class' to follow.
func (p *Parser) parseDecoratedStatement() Statement {
	decorators := p.parseDecoratorList()

	switch p.curToken.Type {
	case lexer.CLASS:
		stmt := p.parseClassDeclaration()
		if classDecl, ok := stmt.(*ClassDeclaration); ok {
			classDecl.Decorators = decorators
		}
		return stmt

	case lexer.ABSTRACT:
		if !p.expectPeek(lexer.CLASS) {
			p.addError(p.curToken, "expected 'class' after 'abstract' in decorated statement")
			return nil
		}
		stmt := p.parseClassDeclaration()
		if classDecl, ok := stmt.(*ClassDeclaration); ok {
			classDecl.IsAbstract = true
			classDecl.Decorators = decorators
		}
		return stmt

	case lexer.EXPORT:
		return p.parseDecoratedExport(decorators)

	default:
		p.addError(p.curToken, "decorators are only valid on class declarations and class elements")
		return nil
	}
}

// parseDecoratedExport handles: @dec export class C {} or @dec export default class C {}
func (p *Parser) parseDecoratedExport(decorators []*Decorator) Statement {
	exportToken := p.curToken
	p.nextToken() // consume 'export'

	if p.curTokenIs(lexer.DEFAULT) {
		p.nextToken() // consume 'default'

		if !p.curTokenIs(lexer.CLASS) && !(p.curTokenIs(lexer.ABSTRACT) && p.peekTokenIs(lexer.CLASS)) {
			p.addError(p.curToken, "decorators are only valid on class declarations")
			return nil
		}

		isAbstract := false
		if p.curTokenIs(lexer.ABSTRACT) {
			isAbstract = true
			p.nextToken() // consume 'abstract'
		}

		// Parse class expression (may be anonymous for export default)
		// parseClassExpression expects curToken to be 'class'
		classExpr := p.parseClassExpression()
		if ce, ok := classExpr.(*ClassExpression); ok && ce != nil {
			ce.Decorators = decorators
			if isAbstract {
				ce.IsAbstract = true
			}
		}

		return &ExportDefaultDeclaration{
			Token:       exportToken,
			Declaration: classExpr,
		}
	}

	// export class C {} or export abstract class C {}
	isAbstract := false
	if p.curTokenIs(lexer.ABSTRACT) {
		isAbstract = true
		p.nextToken() // consume 'abstract'
	}

	if !p.curTokenIs(lexer.CLASS) {
		p.addError(p.curToken, "decorators are only valid on class declarations")
		return nil
	}

	classDecl := p.parseClassDeclaration()
	if cd, ok := classDecl.(*ClassDeclaration); ok {
		cd.Decorators = decorators
		if isAbstract {
			cd.IsAbstract = true
		}
	}

	return &ExportNamedDeclaration{
		Token:       exportToken,
		Declaration: classDecl,
	}
}

// parseDecoratedClassExpression parses a decorated class in expression position.
// e.g., var C = @dec class {}
func (p *Parser) parseDecoratedClassExpression() Expression {
	decorators := p.parseDecoratorList()

	if !p.curTokenIs(lexer.CLASS) {
		p.addError(p.curToken, "decorators are only valid on class expressions")
		return nil
	}

	classExpr := p.parseClassExpression()
	if ce, ok := classExpr.(*ClassExpression); ok && ce != nil {
		ce.Decorators = decorators
	}
	return classExpr
}

// parseDecoratorList parses zero or more decorators: @expr @expr ...
// Returns nil if no decorators are found.
func (p *Parser) parseDecoratorList() []*Decorator {
	var decorators []*Decorator
	for p.curTokenIs(lexer.AT) {
		dec := p.parseDecorator()
		if dec != nil {
			decorators = append(decorators, dec)
		}
	}
	return decorators
}

// parseDecorator parses a single decorator: @ DecoratorExpression
// Per TC39 spec, decorator expressions are restricted to:
//   - @ DecoratorMemberExpression (identifier or dot-chained member access)
//   - @ DecoratorCallExpression (member expression followed by arguments)
//   - @ DecoratorParenthesizedExpression (arbitrary expression in parens)
func (p *Parser) parseDecorator() *Decorator {
	atToken := p.curToken // The '@' token
	p.nextToken()         // consume '@', move to expression start

	var expr Expression

	if p.curTokenIs(lexer.LPAREN) {
		// DecoratorParenthesizedExpression: @(expr)
		p.nextToken() // consume '('
		expr = p.parseExpression(LOWEST)
		if !p.expectPeek(lexer.RPAREN) {
			return nil
		}
		p.nextToken() // consume ')'
	} else {
		// DecoratorMemberExpression: @identifier or @a.b.c
		// DecoratorCallExpression: @identifier() or @a.b.c()
		expr = p.parseDecoratorMemberExpression()
		if expr == nil {
			return nil
		}

		// Check for call expression: DecoratorMemberExpression Arguments
		if p.curTokenIs(lexer.LPAREN) {
			// DecoratorCallExpression - parse the call
			expr = p.parseDecoratorCallExpression(expr)
		}
	}

	return &Decorator{
		Token:      atToken,
		Expression: expr,
	}
}

// parseDecoratorMemberExpression parses a decorator member expression:
//
//	IdentifierReference
//	DecoratorMemberExpression . IdentifierName
//	DecoratorMemberExpression . PrivateIdentifier
func (p *Parser) parseDecoratorMemberExpression() Expression {
	// Must start with an identifier (or contextual keyword usable as identifier)
	if !p.curTokenIs(lexer.IDENT) && !p.isKeywordThatCanBeIdentifier(p.curToken.Type) {
		p.addError(p.curToken, "expected identifier in decorator expression")
		return nil
	}

	var expr Expression = &Identifier{Token: p.curToken, Value: p.curToken.Literal}
	p.nextToken() // consume identifier

	// Parse dot-chained member access: a.b.c
	for p.curTokenIs(lexer.DOT) {
		dotToken := p.curToken
		p.nextToken() // consume '.'

		// After dot, expect identifier name or private identifier
		if p.curTokenIs(lexer.PRIVATE_IDENT) {
			expr = &MemberExpression{
				Token:    dotToken,
				Object:   expr,
				Property: &Identifier{Token: p.curToken, Value: p.curToken.Literal},
			}
			p.nextToken() // consume private identifier
		} else if p.curTokenIs(lexer.IDENT) || p.isKeywordThatCanBeIdentifier(p.curToken.Type) {
			expr = &MemberExpression{
				Token:    dotToken,
				Object:   expr,
				Property: &Identifier{Token: p.curToken, Value: p.curToken.Literal},
			}
			p.nextToken() // consume identifier
		} else {
			p.addError(p.curToken, "expected identifier after '.' in decorator expression")
			return nil
		}
	}

	return expr
}

// parseDecoratorCallExpression parses a decorator call: expr(args)
// The callee has already been parsed as a DecoratorMemberExpression.
func (p *Parser) parseDecoratorCallExpression(callee Expression) Expression {
	// p.curToken is '('
	callExpr := &CallExpression{
		Token:    p.curToken,
		Function: callee,
	}

	callExpr.Arguments = p.parseExpressionList(lexer.RPAREN)
	p.nextToken() // consume ')'

	return callExpr
}

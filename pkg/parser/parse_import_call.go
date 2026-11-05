package parser

import "github.com/nooga/paserati/pkg/lexer"

// parseImportCallLoose parses the arguments of `import(...)` the way tsc
// does, as a plain argument list. curToken is the opening parenthesis. Having
// no argument, a spread argument or more than two arguments is not a syntax
// error; the checker reports TS1450, TS1325 and TS1324 for them.
func (p *Parser) parseImportCallLoose(importToken *lexer.Token) Expression {
	args := p.parseExpressionList(lexer.RPAREN)
	node := &DynamicImportExpression{Token: importToken}
	if args == nil {
		return node
	}
	if len(args) > 0 {
		node.Source = args[0]
	}
	if len(args) > 1 {
		node.Options = args[1]
	}
	if len(args) > 2 {
		node.ExtraArgs = args[2:]
	}
	return node
}

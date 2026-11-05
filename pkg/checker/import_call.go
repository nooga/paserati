package checker

import "github.com/nooga/paserati/pkg/parser"

// checkImportCallGrammar mirrors tsc's checkGrammarImportCallExpression for a
// dynamic import(): the module kind decides whether the call, or a second
// argument, is allowed, and the argument list must be a specifier, optional
// attributes and no spread. Only the first problem is reported.
func (c *Checker) checkImportCallGrammar(node *parser.DynamicImportExpression) {
	switch c.moduleKind {
	case "es6", "es2015":
		c.addErrorWithCode(node, "TS1323", "Dynamic imports are only supported when the '--module' flag is set to 'es2020', 'es2022', 'esnext', 'commonjs', 'amd', 'system', 'umd', 'node16', 'node18', 'node20', or 'nodenext'.")
		return
	case "", "node16", "node18", "node20", "nodenext", "esnext", "preserve":
		// Unset keeps the default module kind, which allows both arguments.
	default:
		if node.Options != nil {
			c.addErrorWithCode(node.Options, "TS1324", "Dynamic imports only support a second argument when the '--module' option is set to 'esnext', 'node16', 'node18', 'node20', 'nodenext', or 'preserve'.")
			return
		}
	}
	// tsc's TS1450 for a wrong argument count is a message, not an error.
	if node.Source == nil || len(node.ExtraArgs) > 0 {
		return
	}
	for _, arg := range []parser.Expression{node.Source, node.Options} {
		if spread, ok := arg.(*parser.SpreadElement); ok {
			c.addErrorWithCode(spread, "TS1325", "Argument of dynamic import cannot be spread element.")
			return
		}
	}
}

// checkDeferredImportClause mirrors tsc's checkGrammarImportClause for
// `import defer`: only a namespace binding is allowed, and only under
// --module esnext or preserve.
func (c *Checker) checkDeferredImportClause(node *parser.ImportDeclaration) {
	if !node.IsDeferred || node.ClauseToken == nil {
		return
	}
	at := &parser.Identifier{Token: node.ClauseToken}
	if len(node.Specifiers) > 0 {
		if _, ok := node.Specifiers[0].(*parser.ImportDefaultSpecifier); ok {
			c.addErrorWithCode(at, "TS18058", "Default imports are not allowed in a deferred import.")
			return
		}
	}
	for _, spec := range node.Specifiers {
		if _, ok := spec.(*parser.ImportNamedSpecifier); ok {
			c.addErrorWithCode(at, "TS18059", "Named imports are not allowed in a deferred import.")
			return
		}
	}
	c.checkDeferredModuleKind(at)
}

// checkDeferredModuleKind reports TS18060 for a deferred import under a
// --module that cannot express it. An unset --module is left alone.
func (c *Checker) checkDeferredModuleKind(node parser.Node) {
	switch c.moduleKind {
	case "", "esnext", "preserve":
		return
	}
	c.addErrorWithCode(node, "TS18060", "Deferred imports are only supported when the '--module' flag is set to 'esnext' or 'preserve'.")
}

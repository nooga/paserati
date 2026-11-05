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

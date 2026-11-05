package checker

import (
	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/parser"
)

// checkImportCallGrammar mirrors tsc's checkGrammarImportCallExpression for a
// dynamic import(): the module kind decides whether the call, or a second
// argument, is allowed, and the argument list must be a specifier, optional
// attributes and no spread. Only the first problem is reported.
func (c *Checker) checkImportCallGrammar(node *parser.DynamicImportExpression) {
	if c.grammarErrorsSuppressed() {
		return
	}
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
	if !node.IsDeferred || node.ClauseToken == nil || c.grammarErrorsSuppressed() {
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
	if c.grammarErrorsSuppressed() {
		return
	}
	switch c.moduleKind {
	case "", "esnext", "preserve":
		return
	}
	c.addErrorWithCode(node, "TS18060", "Deferred imports are only supported when the '--module' flag is set to 'esnext' or 'preserve'.")
}

// checkImportAttributesModule mirrors the module-kind part of tsc's
// checkImportAttributes for an import or export declaration with a `with`
// clause: TS2823 when --module cannot carry attributes, TS2857 when the
// declaration is type-only.
func (c *Checker) checkImportAttributesModule(with *lexer.Token, typeOnly bool, attrs map[string]string) {
	if with == nil || c.grammarErrorsSuppressed() {
		return
	}
	// A type-only import or export may carry a resolution-mode attribute
	// (and then the other attribute rules do not apply).
	if _, ok := attrs["resolution-mode"]; ok && typeOnly {
		return
	}
	at := &parser.Identifier{Token: with}
	switch c.moduleKind {
	case "":
		return
	case "esnext", "node18", "node20", "nodenext", "preserve":
	default:
		c.addErrorWithCode(at, "TS2823", "Import attributes are only supported when the '--module' option is set to 'esnext', 'node18', 'node20', 'nodenext', or 'preserve'.")
		return
	}
	if typeOnly {
		c.addErrorWithCode(at, "TS2857", "Import attributes cannot be used with type-only imports or exports.")
		return
	}
	if c.moduleKind != "esnext" && c.moduleKind != "preserve" && c.inCommonJSFormat() {
		c.addErrorWithCode(at, "TS2856", "Import attributes are not allowed on statements that compile to CommonJS 'require' calls.")
	}
}

// grammarErrorsSuppressed reports whether tsc's grammarErrorOnNode would stay
// silent: it reports nothing in a file that has parse diagnostics.
func (c *Checker) grammarErrorsSuppressed() bool {
	return c.program != nil && c.program.HasParseDiagnostics
}

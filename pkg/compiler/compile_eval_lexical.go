package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
)

// Direct eval's var declarations hoist to the caller's variable environment,
// and EvalDeclarationInstantiation (ECMA-262 19.2.1.3 step 3) rejects one that
// would cross a let/const/class of the same name on the way there. Which names
// those are depends on where the eval call sits, so the compiler tracks the
// lexical declarations of each enclosing scope and records them per call site.

// lexicalDeclNames lists the let/const/class names a statement list declares
// directly (not inside nested blocks).
func lexicalDeclNames(stmts []parser.Statement) []string {
	names := collectLetConstDeclarations(stmts)
	return append(names, collectClassDeclarations(stmts)...)
}

// pushLexicalScope records the lexical names of a scope being entered. It
// returns the function that pops it again.
func (c *Compiler) pushLexicalScope(names []string) func() {
	if len(names) == 0 {
		return func() {}
	}
	c.lexicalScopes = append(c.lexicalScopes, names)
	depth := len(c.lexicalScopes)
	return func() { c.lexicalScopes = c.lexicalScopes[:depth-1] }
}

// recordEvalSiteLexicals notes the lexical names visible at the direct eval
// call about to be emitted at the current code offset.
func (c *Compiler) recordEvalSiteLexicals() {
	var names []string
	for _, scope := range c.lexicalScopes {
		names = append(names, scope...)
	}
	if len(names) == 0 {
		return
	}
	if c.chunk.EvalSiteLexicals == nil {
		c.chunk.EvalSiteLexicals = make(map[int][]string)
	}
	c.chunk.EvalSiteLexicals[len(c.chunk.Code)] = names
}

// checkEvalVarLexicalConflicts reports a SyntaxError for a var or function
// declaration of sloppy direct eval code that names a lexical binding between
// the eval and the caller's variable environment.
func (c *Compiler) checkEvalVarLexicalConflicts(program *parser.Program) {
	if c.callerScopeDesc == nil || len(c.callerScopeDesc.LexicalBindings) == 0 || c.chunk.IsStrict {
		return
	}
	lexical := make(map[string]bool, len(c.callerScopeDesc.LexicalBindings))
	for _, name := range c.callerScopeDesc.LexicalBindings {
		lexical[name] = true
	}
	names := collectVarDeclarations(program.Statements)
	for name := range program.HoistedDeclarations {
		names = append(names, name)
	}
	for _, name := range names {
		if lexical[name] {
			c.addError(program, fmt.Sprintf("SyntaxError: Identifier '%s' has already been declared", name))
			return
		}
	}
}

package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// TS1212/TS1213/TS1214 — "Identifier expected. 'X' is a reserved word in
// strict mode."
//
// The future reserved words below are ordinary identifiers in sloppy mode but
// reserved once strict mode applies. TypeScript reports them from the binder,
// so the diagnostic appears alongside whatever the checker then says about the
// name — usually TS2304, since a reserved word rarely resolves to anything.
//
// The three codes differ only in how they explain why strict mode is in force,
// so the code to use is chosen from where the identifier sits.

// strictModeReservedWords are the ECMAScript future reserved words that are
// only reserved in strict mode. `await` is absent: it is reserved by module
// and async context rather than by strict mode.
var strictModeReservedWords = map[string]bool{
	"implements": true,
	"interface":  true,
	"let":        true,
	"package":    true,
	"private":    true,
	"protected":  true,
	"public":     true,
	"static":     true,
	"yield":      true,
}

// checkStrictModeIdentifier reports TS1212 (or its class/module variants) when
// an identifier used as a name is a word reserved in strict mode.
func (c *Checker) checkStrictModeIdentifier(node *parser.Identifier) {
	if node == nil || !strictModeReservedWords[node.Value] {
		return
	}

	// Class bodies and modules are strict on their own terms, so they report
	// regardless of --alwaysStrict; a plain script only does when the option
	// is on. TypeScript names the reason in the message, which is the only
	// difference between the three codes.
	code, why := errors.TS1212, ""
	switch {
	case c.getCurrentClassName() != "":
		code, why = errors.TS1213, " Class definitions are automatically in strict mode."
	case c.isModule:
		code, why = errors.TS1214, " Modules are automatically in strict mode."
	case !c.alwaysStrict:
		return
	}

	// Narrowing can bring the checker past the same identifier more than once.
	if c.reportedStrictReserved == nil {
		c.reportedStrictReserved = make(map[*parser.Identifier]bool)
	} else if c.reportedStrictReserved[node] {
		return
	}
	c.reportedStrictReserved[node] = true

	c.addErrorWithCode(node, code,
		"Identifier expected. '"+node.Value+"' is a reserved word in strict mode."+why)
}

package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// A name imported with `import type` (or `import { type X }`, `import type X
// = require()`) has no value: using it in an expression is TS1361, as tsc's
// resolveNameHelper reports for an alias whose only declaration on the value
// side is type-only. The use sites tsc accepts (isValidTypeOnlyAliasUseSite)
// are type positions and ambient code, which do not visit the name as a value,
// and `export default X`, which re-exports the alias.
//
// FIXME: TS1362 (a name exported with `export type` and imported plainly) and
// type-only-ness carried through re-exports and default exports need the
// source module's export flags on its record.

// checkTypeOnlyImportUse reports a value use of a type-only import.
func (c *Checker) checkTypeOnlyImportUse(node *parser.Identifier) {
	if c.moduleEnv == nil || c.typeOnlyUseAllowed || c.inAmbientNamespace {
		return
	}
	binding := c.moduleEnv.ImportedNames[node.Value]
	if binding == nil || !binding.IsTypeOnly {
		return
	}
	if c.definingEnv(node.Value) != c.moduleEnv.Environment {
		return // a local declaration shadows the import
	}
	c.addErrorWithCode(node, errors.TS1361, fmt.Sprintf("'%s' cannot be used as a value because it was imported using 'import type'.", node.Value))
}

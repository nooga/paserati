package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// globalThisType is `typeof globalThis`: an object type with a property for
// each global value declared so far (the built-ins, `declare global` blocks
// and script-file declarations). @types/node keys its web globals on it -
// `typeof globalThis extends { onmessage: any } ? {} : undici.Request` picks
// the undici types unless the DOM lib declared onmessage - so it cannot be
// `any`, which would match both branches.
func (c *Checker) globalThisType() types.Type {
	obj := types.NewObjectType()
	seen := map[*Environment]bool{}
	add := func(env *Environment) {
		for ; env != nil; env = env.outer {
			if seen[env] {
				return
			}
			seen[env] = true
			for name, sym := range env.symbols {
				if sym.Type == nil || name == "globalThis" || c.isBlockScopedGlobal(env, name) {
					continue
				}
				if _, exists := obj.Props()[name]; !exists {
					obj.SetProperty(name, sym.Type)
				}
			}
		}
	}
	add(c.globalScope)
	add(c.builtinEnv)
	obj.SetProperty("globalThis", obj)
	obj.DisplayName = "typeof globalThis"
	return obj
}

// isBuiltinGlobalThis reports whether name resolves to the built-in globalThis
// rather than a local binding that shadows it.
func (c *Checker) isBuiltinGlobalThis(name string, t types.Type) bool {
	return name == "globalThis" && t == types.Any
}

// isBlockScopedGlobal reports whether name is a let, const or class declared
// in env: such globals are not properties of globalThis.
func (c *Checker) isBlockScopedGlobal(env *Environment, name string) bool {
	_, ok := c.blockScoped[env][name]
	return ok
}

// checkGlobalThisMember reports TS2339 for globalThis.name (or
// globalThis["name"]) naming a block-scoped global.
func (c *Checker) checkGlobalThisMember(obj parser.Expression, name string, at parser.Node) bool {
	id, ok := obj.(*parser.Identifier)
	if !ok || id.Value != "globalThis" || c.globalScope == nil {
		return false
	}
	if t, _, found := c.env.Resolve("globalThis"); !found || !c.isBuiltinGlobalThis("globalThis", t) {
		return false
	}
	if !c.isBlockScopedGlobal(c.globalScope, name) {
		return false
	}
	c.addErrorWithCode(at, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type 'typeof globalThis'.", name))
	return true
}

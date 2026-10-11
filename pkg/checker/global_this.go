package checker

import "github.com/nooga/paserati/pkg/types"

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
				if sym.Type == nil || name == "globalThis" {
					continue
				}
				if _, exists := obj.Properties[name]; !exists {
					obj.SetProperty(name, sym.Type)
				}
			}
		}
	}
	add(c.globalScope)
	add(globalEnvironment.Load())
	obj.SetProperty("globalThis", obj)
	obj.DisplayName = "typeof globalThis"
	return obj
}

// isBuiltinGlobalThis reports whether name resolves to the built-in globalThis
// rather than a local binding that shadows it.
func (c *Checker) isBuiltinGlobalThis(name string, t types.Type) bool {
	return name == "globalThis" && t == types.Any
}

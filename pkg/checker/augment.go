package checker

import (
	"sort"

	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// A module augmentation, `declare module "m" { ... }` in a module, adds its
// declarations to the module "m" resolves to: an interface merges into the
// interface "m" exports (vitest adds its snapshot matchers to Assertion this
// way), and a new declaration becomes an export of "m".
//
// The augmentation's body is checked in its own file, as an ambient
// namespace; once that file is checked its members are copied into the
// target's exports. The target is often still being checked then (vitest's
// index imports the chunk that augments "vitest"), so the augmentation waits
// in the program's registry until the target's exports exist.

// isModuleAugmentation reports whether node, an ambient module declaration at
// the top level of the file being checked, augments another module.
func (c *Checker) isModuleAugmentation(node *parser.NamespaceDeclaration) bool {
	return node.AmbientModule && node.Name != nil && node.Name.Value != "global" &&
		c.namespaceBodyDepth == 0 && c.moduleLoader != nil && c.program != nil &&
		programHasModuleSyntax(c.program)
}

type moduleAugmentation struct {
	specifier string
	ns        *types.NamespaceType
}

// applyModuleAugmentations merges the file's augmentations into their target
// modules, or leaves them in the registry for targets still being checked.
func (c *Checker) applyModuleAugmentations() {
	augs := c.augmentations
	c.augmentations = nil
	if len(augs) == 0 {
		return
	}
	from := ""
	if c.moduleEnv != nil {
		from = c.moduleEnv.ModulePath
	} else if c.source != nil {
		from = c.source.Path
	}
	for _, aug := range augs {
		loaded, err := c.moduleLoader.LoadModule(aug.specifier, from)
		if err != nil {
			continue
		}
		rec, ok := loaded.(*modules.ModuleRecord)
		if !ok || rec == nil {
			continue
		}
		if rec.Exports != nil {
			c.mergeAugmentation(rec.Exports, aug.ns)
			continue
		}
		if g := c.sharedGlobals; g != nil && rec.ResolvedPath != "" {
			g.mu.Lock()
			if g.augmentations == nil {
				g.augmentations = map[string][]*types.NamespaceType{}
			}
			g.augmentations[rec.ResolvedPath] = append(g.augmentations[rec.ResolvedPath], aug.ns)
			g.mu.Unlock()
		}
	}
}

// takeAugmentations removes and returns the augmentations waiting for the
// module at path.
func (c *Checker) takeAugmentations(path string) []*types.NamespaceType {
	g := c.sharedGlobals
	if g == nil || path == "" {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	augs := g.augmentations[path]
	delete(g.augmentations, path)
	return augs
}

// mergeAugmentation adds an augmentation's declarations to a module's
// exports: interfaces merge into the exported interfaces of the same name in
// place, so every module holding them sees the new members; other names are
// added as exports when the module has none of that name.
func (c *Checker) mergeAugmentation(exports map[string]types.Type, ns *types.NamespaceType) {
	for _, name := range sortedKeys(ns.TypeMembers) {
		src := ns.TypeMembers[name]
		target, exists := exports[name]
		if !exists {
			exports[name] = src
			continue
		}
		c.mergeInterfaceInto(target, src)
	}
	for _, name := range ns.ValueShape.PropertyNames() {
		if _, exists := exports[name]; !exists {
			exports[name] = ns.ValueShape.Props()[name]
		}
	}
}

// mergeInterfaceInto copies the members of the interface src into the
// interface target (both generic with the same parameters, or both not).
// Members refer to type parameters by name, which merged declarations share.
func (c *Checker) mergeInterfaceInto(target, src types.Type) {
	switch tg := target.(type) {
	case *types.GenericType:
		sg, ok := src.(*types.GenericType)
		if !ok || len(sg.TypeParameters) != len(tg.TypeParameters) {
			return
		}
		for i, p := range tg.TypeParameters {
			if sg.TypeParameters[i].Name != p.Name {
				return
			}
		}
		dst, ok1 := tg.Body.(*types.ObjectType)
		from, ok2 := sg.Body.(*types.ObjectType)
		if !ok1 || !ok2 {
			return
		}
		copyInterfaceMembers(dst, from)
		if c.instCache != nil {
			c.refillInstances(tg)
		}
	case *types.ObjectType:
		if from, ok := src.(*types.ObjectType); ok {
			copyInterfaceMembers(tg, from)
		}
	}
}

// copyInterfaceMembers merges the declaration src into dst: new properties
// are added, a method declared in both gains src's overloads, and src's call,
// construct and index signatures are appended.
func copyInterfaceMembers(dst, src *types.ObjectType) {
	if dst == src {
		return
	}
	for _, name := range src.PropertyNames() {
		prop := src.Props()[name]
		if existing, exists := dst.Props()[name]; exists {
			if merged, ok := mergeInterfaceFunctionMemberTypes(existing, prop); ok {
				dst.SetProperty(name, merged)
			}
			continue
		}
		dst.SetProperty(name, prop)
		if src.OptionalProps()[name] {
			dst.OptionalProps()[name] = true
		}
		setReadonlyProperty(dst, name, src.ReadOnlyProps()[name])
	}
	if sigs := src.CallSigs(); len(sigs) > 0 {
		dst.Materialize()
		dst.CallSignatures = append(dst.CallSigs(), sigs...)
	}
	if sigs := src.ConstructSigs(); len(sigs) > 0 {
		dst.Materialize()
		dst.ConstructSignatures = append(dst.ConstructSigs(), sigs...)
	}
	if sigs := src.IndexSigs(); len(sigs) > 0 {
		dst.Materialize()
		dst.IndexSignatures = append(dst.IndexSigs(), sigs...)
	}
}

func sortedKeys(m map[string]types.Type) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

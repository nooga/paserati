package checker

import (
	"sync"

	"github.com/nooga/paserati/pkg/types"
)

// GlobalDeclarations collects the global declarations of the files a
// program checks, shared by the per-module checkers: `declare global { ... }`
// blocks and the top-level declarations of script files (a declaration file
// such as vitest/globals, or a script without imports or exports). tsc sees
// every file's globals everywhere; here a file sees those of the files
// checked before it, so a host checks the global files first.
type GlobalDeclarations struct {
	mu     sync.Mutex
	values map[string]types.Type
	types  map[string]types.Type

	lateBases []lateBase
	derived   map[*types.ObjectType][]*types.ObjectType

	instCache *instantiationCache

	// augmentations are the module augmentations waiting for their target
	// module's exports, by the target's resolved path (see augment.go).
	augmentations map[string][]*types.NamespaceType
}

// NewGlobalDeclarations returns an empty registry.
func NewGlobalDeclarations() *GlobalDeclarations {
	return &GlobalDeclarations{
		values:    map[string]types.Type{},
		types:     map[string]types.Type{},
		instCache: newInstantiationCache(),
	}
}

// SetSharedGlobals makes the checker see the registry's declarations as
// globals and publish its file's global declarations to it.
func (c *Checker) SetSharedGlobals(g *GlobalDeclarations) {
	c.sharedGlobals = g
	c.instCache = g.instCache
}

// importSharedGlobals defines the registry's declarations in env (the global
// scope). They replace env's own: a registry entry is a global a file
// declared or merged into (`declare global { interface EventTarget {...} }`
// over the any-typed lib stub, or an augmented built-in interface), so it is
// the more complete declaration.
func (c *Checker) importSharedGlobals(env *Environment) {
	g := c.sharedGlobals
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for name, t := range g.types {
		env.typeAliases[name] = t
	}
	for name, t := range g.values {
		if sym, exists := env.symbols[name]; exists {
			sym.Type = t
			env.symbols[name] = sym
		} else {
			env.Define(name, t, false)
		}
	}
}

// globalsSnapshot records what env declares, so the declarations a file adds
// or replaces can be published after it is checked.
type globalsSnapshot struct {
	values map[string]types.Type
	types  map[string]types.Type
}

func snapshotGlobals(env *Environment) globalsSnapshot {
	s := globalsSnapshot{values: make(map[string]types.Type, len(env.symbols)), types: make(map[string]types.Type, len(env.typeAliases))}
	for name, sym := range env.symbols {
		s.values[name] = sym.Type
	}
	for name, t := range env.typeAliases {
		s.types[name] = t
	}
	return s
}

// publishGlobals adds the global declarations env gained or replaced since
// the snapshot to the registry. (Merging into an interface mutates it in
// place, so an augmented registry entry needs no republishing.)
func (c *Checker) publishGlobals(env *Environment, before globalsSnapshot) {
	g := c.sharedGlobals
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for name, sym := range env.symbols {
		if prev, seen := before.values[name]; sym.Type != nil && (!seen || prev != sym.Type) {
			g.values[name] = sym.Type
		}
	}
	for name, t := range env.typeAliases {
		if prev, seen := before.types[name]; t != nil && (!seen || prev != t) {
			g.types[name] = t
		}
	}
	g.resolveLateBases()
}

// lateBase is an interface whose extends clause names a global that was still
// the any-typed lib stub when it was declared: @types/node's
// `interface AbortSignal extends EventTarget` in web-globals/abortcontroller.d.ts
// is checked before web-globals/events.d.ts declares EventTarget. tsc merges
// every file's globals before checking; here the base's members are copied
// in once a file publishes the declaration.
type lateBase struct {
	derived *types.ObjectType
	name    string
}

// recordLateBase notes that derived extends the global name, not declared yet.
func (c *Checker) recordLateBase(derived *types.ObjectType, name string) {
	g := c.sharedGlobals
	if g == nil || derived == nil {
		return
	}
	g.mu.Lock()
	g.lateBases = append(g.lateBases, lateBase{derived: derived, name: name})
	g.mu.Unlock()
}

// recordDerived notes that derived copied base's members, so members base
// gains late reach derived too.
func (c *Checker) recordDerived(base, derived *types.ObjectType) {
	g := c.sharedGlobals
	if g == nil || base == nil || derived == nil || base == derived {
		return
	}
	g.mu.Lock()
	if g.derived == nil {
		g.derived = map[*types.ObjectType][]*types.ObjectType{}
	}
	g.derived[base] = append(g.derived[base], derived)
	g.mu.Unlock()
}

// resolveLateBases copies the members of the globals pending interfaces name
// once they are declared. Called with g.mu held.
func (g *GlobalDeclarations) resolveLateBases() {
	if len(g.lateBases) == 0 {
		return
	}
	pending := g.lateBases[:0]
	for _, lb := range g.lateBases {
		base, ok := types.GetEffectiveType(g.types[lb.name]).(*types.ObjectType)
		if !ok {
			pending = append(pending, lb)
			continue
		}
		g.inheritLate(base, lb.derived, map[*types.ObjectType]bool{})
		g.derived[base] = append(g.derived[base], lb.derived)
	}
	g.lateBases = pending
}

// inheritLate gives derived (and what derives from it) the members of base it
// does not declare itself.
func (g *GlobalDeclarations) inheritLate(base, derived *types.ObjectType, seen map[*types.ObjectType]bool) {
	if seen[derived] || base == derived {
		return
	}
	seen[derived] = true
	added := false
	for _, name := range base.PropertyNames() {
		if _, exists := derived.Props()[name]; exists {
			continue
		}
		derived.SetProperty(name, base.Props()[name])
		if base.OptionalProps()[name] {
			derived.OptionalProps()[name] = true
		}
		setReadonlyProperty(derived, name, base.ReadOnlyProps()[name])
		added = true
	}
	if !added {
		return
	}
	for _, next := range g.derived[derived] {
		g.inheritLate(derived, next, seen)
	}
}

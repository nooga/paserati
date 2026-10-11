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
}

// NewGlobalDeclarations returns an empty registry.
func NewGlobalDeclarations() *GlobalDeclarations {
	return &GlobalDeclarations{values: map[string]types.Type{}, types: map[string]types.Type{}}
}

// SetSharedGlobals makes the checker see the registry's declarations as
// globals and publish its file's global declarations to it.
func (c *Checker) SetSharedGlobals(g *GlobalDeclarations) {
	c.sharedGlobals = g
}

// importSharedGlobals defines the registry's declarations in env (the global
// scope) where env has no declaration of that name yet.
func (c *Checker) importSharedGlobals(env *Environment) {
	g := c.sharedGlobals
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for name, t := range g.types {
		if _, exists := env.typeAliases[name]; !exists {
			env.DefineTypeAlias(name, t)
		}
	}
	for name, t := range g.values {
		if _, exists := env.symbols[name]; !exists {
			env.Define(name, t, false)
		}
	}
}

// globalsSnapshot records the names env declares, so the ones a file adds
// can be published after it is checked.
type globalsSnapshot struct {
	values map[string]bool
	types  map[string]bool
}

func snapshotGlobals(env *Environment) globalsSnapshot {
	s := globalsSnapshot{values: map[string]bool{}, types: map[string]bool{}}
	for name := range env.symbols {
		s.values[name] = true
	}
	for name := range env.typeAliases {
		s.types[name] = true
	}
	return s
}

// publishGlobals adds the global declarations env gained since the snapshot
// to the registry.
func (c *Checker) publishGlobals(env *Environment, before globalsSnapshot) {
	g := c.sharedGlobals
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for name, sym := range env.symbols {
		if !before.values[name] && sym.Type != nil {
			if _, exists := g.values[name]; !exists {
				g.values[name] = sym.Type
			}
		}
	}
	for name, t := range env.typeAliases {
		if !before.types[name] && t != nil {
			if _, exists := g.types[name]; !exists {
				g.types[name] = t
			}
		}
	}
}

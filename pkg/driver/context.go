package driver

import (
	"fmt"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/vm"
)

// Context is an isolated realm on a Paserati session: its own global object,
// intrinsics (Object, Array, Error, ...) and global variables, backed by the
// session's VM, compiler and module loader. It is the supported way to run many
// independent pieces of code - a request, a script call - on one long-lived
// session without paying for a whole new engine each time, and without one
// run's globals, prototype changes or leftovers being visible to the next.
//
// Lifetime: a Context is ordinary garbage-collected memory. Dropping every
// reference to it (and to values it produced) frees its realm; there is nothing
// to close. Values created in one Context must not be handed to another unless
// you mean to share them.
//
// A Context is not safe for concurrent use, and neither is the session it
// belongs to: run Contexts of one session one at a time (a pool of sessions,
// each used by one goroutine at a time, is the way to run in parallel).
//
// ES modules are per Context as well: importing a module in a Context evaluates
// it there, in a private instance of the module's compiled code, with its own
// top-level state, classes and namespace object. Only the expensive part is
// shared - the session parses, type-checks and compiles each module once, on
// first import in any Context - so a call that imports a large graph costs
// instantiating and running it, not rebuilding it. Native modules (DeclareModule)
// are built per Context too, unless declared Shared().
//
// Per-call host state (credentials, a request scope) belongs in the Context's
// own globals - see SetGlobal and DefineNativeGlobal. Natives bound that way
// exist only in that Context, so a pooled session cannot leak them into the
// next call's.
type Context struct {
	p     *Paserati
	realm *vm.Realm
}

// NewContext creates a Context with the standard builtins.
func (p *Paserati) NewContext() (*Context, error) {
	return p.NewContextWithInitializers(builtins.GetStandardInitializers())
}

// NewContextWithInitializers creates a Context whose globals come from the given
// builtin initializers. Pass the same set the session was created with (see
// NewPaseratiWithInitializers) so global slots line up with compiled code.
func (p *Paserati) NewContextWithInitializers(initializers []builtins.BuiltinInitializer) (*Context, error) {
	if p.vmInstance == nil {
		return nil, fmt.Errorf("session has been cleaned up")
	}
	realm := p.vmInstance.CreateRealm()
	if err := p.InitializeRealmBuiltins(realm, initializers); err != nil {
		return nil, err
	}
	return &Context{p: p, realm: realm}, nil
}

// Realm returns the underlying realm, for hosts that need the low-level API.
func (c *Context) Realm() *vm.Realm { return c.realm }

// Global returns the Context's global object (its globalThis).
func (c *Context) Global() vm.Value { return vm.NewValueFromPlainObject(c.realm.GlobalObject) }

// SetGlobal defines or overwrites a global variable in this Context only.
func (c *Context) SetGlobal(name string, value vm.Value) {
	c.realm.SetGlobal(name, value)
	if c.realm.GlobalObject != nil {
		c.realm.GlobalObject.SetOwn(name, value)
	}
}

// DefineNativeGlobal binds a Go function as a global in this Context only. fn
// is free to close over per-call state.
func (c *Context) DefineNativeGlobal(name string, arity int, fn func(args []vm.Value) (vm.Value, error)) {
	c.SetGlobal(name, vm.NewNativeFunction(arity, false, name, fn))
}

// GetGlobal reads a global variable from this Context.
func (c *Context) GetGlobal(name string) (vm.Value, bool) { return c.realm.GetGlobal(name) }

// RunCode compiles and runs source in this Context, like Paserati.RunCode.
func (c *Context) RunCode(sourceCode string, options RunOptions) (vm.Value, []errors.PaseratiError) {
	var (
		v    vm.Value
		errs []errors.PaseratiError
	)
	c.p.vmInstance.WithRealm(c.realm, func() { v, errs = c.p.RunCode(sourceCode, options) })
	return v, errs
}

// RunProgram runs a precompiled Program in this Context, like
// Paserati.RunProgram.
func (c *Context) RunProgram(prog *Program) (vm.Value, []errors.PaseratiError) {
	var (
		v    vm.Value
		errs []errors.PaseratiError
	)
	c.p.vmInstance.WithRealm(c.realm, func() { v, errs = c.p.RunProgram(prog) })
	return v, errs
}

// Call invokes a function value (typically one a script returned) in this
// Context.
func (c *Context) Call(fn, this vm.Value, args ...vm.Value) (vm.Value, error) {
	var (
		v   vm.Value
		err error
	)
	c.p.vmInstance.WithRealm(c.realm, func() { v, err = c.p.vmInstance.Call(fn, this, args) })
	return v, err
}

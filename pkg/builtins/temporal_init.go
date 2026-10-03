package builtins

import "github.com/nooga/paserati/pkg/vm"

// InitRuntime installs the Temporal namespace: an object tagged "Temporal"
// that every type registered with registerTemporalInstaller adds itself to.
func (t *TemporalInitializer) InitRuntime(ctx *RuntimeContext) error {
	ns := vm.NewObject(ctx.VM.ObjectPrototype).AsPlainObject()
	intlDefineToStringTag(ctx.VM, ns, "Temporal")
	r := &temporalRealm{
		vm:     ctx.VM,
		ns:     ns,
		protos: map[string]*vm.PlainObject{},
		ctors:  map[string]vm.Value{},
	}
	for _, install := range temporalInstallers {
		if err := install(r); err != nil {
			return err
		}
	}
	return ctx.DefineGlobal("Temporal", vm.NewValueFromPlainObject(ns))
}

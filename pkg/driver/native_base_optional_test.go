package driver

import (
	"testing"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// A class extending a native base whose hook is optional may declare the hook
// itself and call it: the class's declaration replaces the optional one.
func TestNativeBaseOptionalHookOverridden(t *testing.T) {
	p := NewPaserati()
	defer p.Cleanup()
	p.DeclareModule("base", func(m *ModuleBuilder) {
		v := p.GetVM()
		inst := types.NewObjectType().WithOptionalProperty("check", types.NewSimpleFunction(nil, types.Boolean))
		ctorT := types.NewObjectType().WithConstructSignature(&types.Signature{ReturnType: inst})
		proto := vm.NewObject(v.ObjectPrototype)
		ctor := vm.NewConstructorWithProps(0, false, "Base", func([]vm.Value) (vm.Value, error) { return vm.NewObject(proto), nil })
		ctor.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", proto)
		m.Const("Base", ctor)
		m.Type("Base", ctorT)
	})
	v, errs := p.RunCode(`import { Base } from "base";
class Impl extends Base {
  check(): boolean { return true; }
  run(): boolean { return this.check(); }
}
new Impl().run();`, RunOptions{ModuleName: "/m.ts", Filename: "/m.ts"})
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if v.ToString() != "true" {
		t.Fatalf("got %s", v.ToString())
	}
}

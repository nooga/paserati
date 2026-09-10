package builtins

import (
	"github.com/nooga/paserati/pkg/vm"
)

// defineSpeciesAccessor installs the well-known `get [Symbol.species]`
// accessor on a constructor's property table.
//
// Per spec (e.g. ES2025 23.1.2.5 for Array, 23.2.2.4 for %TypedArray%) every
// species-aware constructor carries an accessor whose getter simply returns
// its `this`, with { set: undefined, enumerable: false, configurable: true }.
// Returning `this` rather than a captured constructor is what makes the
// property inherit correctly: `Uint8Array[Symbol.species]` finds the getter on
// %TypedArray% but answers Uint8Array, and `class Foo extends Uint8Array {}`
// answers Foo. For that reason the accessor belongs on the intrinsic only -
// installing a copy on each TypedArray subclass would both break the subclass
// case and wrongly show up in Object.getOwnPropertySymbols(Uint8Array).
func defineSpeciesAccessor(vmInstance *vm.VM, props *vm.PlainObject) {
	if props == nil || vmInstance.SymbolSpecies.Type() != vm.TypeSymbol {
		return
	}
	getter := vm.NewNativeFunction(0, false, "get [Symbol.species]", func(args []vm.Value) (vm.Value, error) {
		return vmInstance.GetThis(), nil
	})
	enumerable, configurable := false, true
	props.DefineAccessorPropertyByKey(
		vm.NewSymbolKey(vmInstance.SymbolSpecies),
		getter, true,
		vm.Undefined, false,
		&enumerable, &configurable,
	)
}

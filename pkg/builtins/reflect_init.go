package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type ReflectInitializer struct{}

func (r *ReflectInitializer) Name() string  { return "Reflect" }
func (r *ReflectInitializer) Priority() int { return PriorityObject + 1 }

func (r *ReflectInitializer) InitTypes(ctx *TypeContext) error {
	// Minimal typing: Reflect.ownKeys(any): any[]
	reflectType := types.NewObjectType().
		WithProperty("ownKeys", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.Any}))
	return ctx.DefineGlobal("Reflect", reflectType)
}

func (r *ReflectInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM
	// Create Reflect object
	reflectObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Implement Reflect.ownKeys by delegating to Object.__ownKeys (strings then symbols)
	ownKeysFn := vm.NewNativeFunction(1, false, "ownKeys", func(args []vm.Value) (vm.Value, error) {
		objVal := vm.Undefined
		if len(args) > 0 {
			objVal = args[0]
		}
		// Get Object.__ownKeys
		objCtor, _ := vmInstance.GetGlobal("Object")
		if nfp := objCtor.AsNativeFunctionWithProps(); nfp != nil {
			if f, ok := nfp.Properties.GetOwn("__ownKeys"); ok {
				return vmInstance.Call(f, vm.Undefined, []vm.Value{objVal})
			}
		}
		return vm.NewArray(), nil
	})
	reflectObj.SetOwn("ownKeys", ownKeysFn)

	return ctx.DefineGlobal("Reflect", vm.NewValueFromPlainObject(reflectObj))
}

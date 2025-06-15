package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"time"
)

type DateInitializer struct{}

func (d *DateInitializer) Name() string {
	return "Date"
}

func (d *DateInitializer) Priority() int {
	return 40 // After core objects
}

func (d *DateInitializer) InitTypes(ctx *TypeContext) error {
	// Create Date constructor type
	dateConstructorType := types.NewSimpleFunction([]types.Type{}, types.String).
		WithProperty("now", types.NewSimpleFunction([]types.Type{}, types.Number))
	
	return ctx.DefineGlobal("Date", dateConstructorType)
}

func (d *DateInitializer) InitRuntime(ctx *RuntimeContext) error {
	// Create Date object with static methods
	dateObj := vm.NewObject(vm.Null).AsPlainObject()
	
	// Add Date.now() method
	dateObj.SetOwn("now", vm.NewNativeFunction(0, false, "now", func(args []vm.Value) vm.Value {
		// Return current time in milliseconds since Unix epoch
		return vm.NumberValue(float64(time.Now().UnixMilli()))
	}))
	
	return ctx.DefineGlobal("Date", vm.NewValueFromPlainObject(dateObj))
}
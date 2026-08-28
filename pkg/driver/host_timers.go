package driver

import (
	"time"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// HostTimerInitializer provides opt-in Node-style nextTick/setTimeout globals
// for embed hosts (e.g. noderati). Not part of standard builtins.
type HostTimerInitializer struct{}

// NewHostTimerInitializer returns a BuiltinInitializer for host timer globals.
func NewHostTimerInitializer() builtins.BuiltinInitializer {
	return &HostTimerInitializer{}
}

func (h *HostTimerInitializer) Name() string {
	return "host-timers"
}

func (h *HostTimerInitializer) Priority() int {
	return 400 // after standard builtins and process (300)
}

func (h *HostTimerInitializer) InitTypes(ctx *builtins.TypeContext) error {
	looseFn := types.NewSimpleFunction([]types.Type{types.Any}, types.Any)
	if err := ctx.DefineGlobal("nextTick", looseFn); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("setTimeout", types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Number)); err != nil {
		return err
	}
	return ctx.DefineGlobal("clearTimeout", types.NewSimpleFunction([]types.Type{types.Number}, types.Undefined))
}

func (h *HostTimerInitializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	vmInstance := ctx.VM
	rt := vmInstance.GetAsyncRuntime()

	nextTickFn := vm.NewNativeFunction(1, true, "nextTick", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 || !args[0].IsCallable() {
			return vm.Undefined, nil
		}
		fn := args[0]
		fnArgs := args[1:]
		rt.ScheduleNextTick(func() {
			_, _ = vmInstance.Call(fn, vm.Undefined, fnArgs)
		})
		return vm.Undefined, nil
	})

	setTimeoutFn := vm.NewNativeFunction(1, true, "setTimeout", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 || !args[0].IsCallable() {
			return vm.NumberValue(0), nil
		}
		fn := args[0]
		delayMs := 0.0
		if len(args) > 1 {
			delayMs = args[1].ToFloat()
			if delayMs < 0 {
				delayMs = 0
			}
		}
		fnArgs := args[2:]
		id := rt.ScheduleTimer(time.Duration(delayMs)*time.Millisecond, func() {
			_, _ = vmInstance.Call(fn, vm.Undefined, fnArgs)
		})
		return vm.NumberValue(float64(id)), nil
	})

	clearTimeoutFn := vm.NewNativeFunction(1, false, "clearTimeout", func(args []vm.Value) (vm.Value, error) {
		if len(args) > 0 && args[0].IsNumber() {
			rt.CancelTimer(uint64(args[0].ToFloat()))
		}
		return vm.Undefined, nil
	})

	if err := ctx.DefineGlobal("nextTick", nextTickFn); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("setTimeout", setTimeoutFn); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("clearTimeout", clearTimeoutFn); err != nil {
		return err
	}

	if processVal, ok := vmInstance.GetGlobal("process"); ok && processVal.IsObject() {
		processVal.AsPlainObject().SetOwn("nextTick", nextTickFn)
	}

	return nil
}

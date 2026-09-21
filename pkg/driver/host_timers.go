package driver

import (
	"fmt"
	"os"
	"time"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// osExit is os.Exit, overridden in tests so they can observe the exit
// request (code, and that it happened) without killing the test binary.
var osExit = os.Exit

// reportUncaughtTimerException reports an exception thrown from a
// setTimeout/nextTick callback the way real Node crashes the process for an
// uncaught exception escaping an event-loop callback - there's no catching JS
// context above these, so unlike a thrown exception inside a normal call
// stack, it can never be caught anywhere and must terminate the process
// (#484). Without this, vmInstance.Call's error return was discarded and the
// process kept running silently, as if the throw never happened.
func reportUncaughtTimerException(vmInstance *vm.VM, err error) {
	fmt.Fprintln(os.Stderr, vmInstance.FormatUncaughtCallError(err))
	osExit(1)
}

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
			if _, err := vmInstance.Call(fn, vm.Undefined, fnArgs); err != nil {
				reportUncaughtTimerException(vmInstance, err)
			}
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
			if _, err := vmInstance.Call(fn, vm.Undefined, fnArgs); err != nil {
				reportUncaughtTimerException(vmInstance, err)
			}
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

package driver

import (
	"fmt"
	"os"
	"runtime"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// ProcessInitializer sets up the Node.js-compatible process global
// This is not standard ECMAScript but needed for compatibility with Node.js scripts
type ProcessInitializer struct {
	argv []string
}

// NewProcessInitializer creates a new ProcessInitializer with the given argv
func NewProcessInitializer(argv []string) *ProcessInitializer {
	return &ProcessInitializer{argv: argv}
}

func (p *ProcessInitializer) Name() string {
	return "process"
}

func (p *ProcessInitializer) Priority() int {
	return 300 // After standard builtins
}

func (p *ProcessInitializer) InitTypes(ctx *builtins.TypeContext) error {
	// Create process type with common properties
	processType := types.NewObjectType().
		WithProperty("argv", &types.ArrayType{ElementType: types.String}).
		WithProperty("platform", types.String).
		WithProperty("version", types.String).
		WithProperty("pid", types.Number).
		WithProperty("env", types.Any).
		WithProperty("execArgv", &types.ArrayType{ElementType: types.String}).
		WithProperty("cwd", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("exit", types.NewSimpleFunction([]types.Type{types.Number}, types.Undefined)).
		WithProperty("stdout", types.Any).
		WithProperty("stderr", types.Any)

	return ctx.DefineGlobal("process", processType)
}

func (p *ProcessInitializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	vmInstance := ctx.VM

	// Create argv array
	argvArray := vm.NewArray()
	arr := argvArray.AsArray()
	for _, arg := range p.argv {
		arr.Append(vm.NewString(arg))
	}

	// Create execArgv (empty for paserati)
	execArgvArray := vm.NewArray()

	// Create env object from os.Environ()
	envObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	for _, env := range os.Environ() {
		for i := 0; i < len(env); i++ {
			if env[i] == '=' {
				key := env[:i]
				value := env[i+1:]
				envObj.SetOwn(key, vm.NewString(value))
				break
			}
		}
	}

	// Create stdout object
	stdoutObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	stdoutObj.SetOwn("write", vm.NewNativeFunction(1, false, "write", func(args []vm.Value) (vm.Value, error) {
		if len(args) > 0 {
			fmt.Print(args[0].ToString())
		}
		return vm.True, nil
	}))
	stdoutObj.SetOwn("isTTY", vm.BooleanValue(true))
	stdoutObj.SetOwn("columns", vm.IntegerValue(80))

	// Create stderr object
	stderrObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	stderrObj.SetOwn("write", vm.NewNativeFunction(1, false, "write", func(args []vm.Value) (vm.Value, error) {
		if len(args) > 0 {
			fmt.Fprint(os.Stderr, args[0].ToString())
		}
		return vm.True, nil
	}))
	stderrObj.SetOwn("isTTY", vm.BooleanValue(true))

	// Create process object
	processObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	processObj.SetOwn("argv", argvArray)
	processObj.SetOwn("execArgv", execArgvArray)
	processObj.SetOwn("platform", vm.NewString(runtime.GOOS))
	processObj.SetOwn("version", vm.NewString("v18.0.0")) // Pretend to be Node 18
	processObj.SetOwn("pid", vm.IntegerValue(int32(os.Getpid())))
	processObj.SetOwn("env", vm.NewValueFromPlainObject(envObj))
	processObj.SetOwn("stdout", vm.NewValueFromPlainObject(stdoutObj))
	processObj.SetOwn("stderr", vm.NewValueFromPlainObject(stderrObj))
	processObj.SetOwn("browser", vm.False) // Not a browser, we're Node-like

	// process.cwd()
	processObj.SetOwn("cwd", vm.NewNativeFunction(0, false, "cwd", func(args []vm.Value) (vm.Value, error) {
		cwd, err := os.Getwd()
		if err != nil {
			return vm.NewString(""), nil
		}
		return vm.NewString(cwd), nil
	}))

	// process.exit(code)
	processObj.SetOwn("exit", vm.NewNativeFunction(1, false, "exit", func(args []vm.Value) (vm.Value, error) {
		code := 0
		if len(args) > 0 && args[0].IsNumber() {
			code = int(args[0].ToFloat())
		}
		os.Exit(code)
		return vm.Undefined, nil
	}))

	// process.nextTick - stub (executes immediately since we don't have event loop)
	processObj.SetOwn("nextTick", vm.NewNativeFunction(1, false, "nextTick", func(args []vm.Value) (vm.Value, error) {
		if len(args) > 0 && args[0].IsCallable() {
			_, _ = vmInstance.Call(args[0], vm.Undefined, args[1:])
		}
		return vm.Undefined, nil
	}))

	// process.memoryUsage() - stub
	processObj.SetOwn("memoryUsage", vm.NewNativeFunction(0, false, "memoryUsage", func(args []vm.Value) (vm.Value, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		result.SetOwn("heapUsed", vm.NumberValue(float64(m.HeapAlloc)))
		result.SetOwn("heapTotal", vm.NumberValue(float64(m.HeapSys)))
		result.SetOwn("rss", vm.NumberValue(float64(m.Sys)))
		return vm.NewValueFromPlainObject(result), nil
	}))

	return ctx.DefineGlobal("process", vm.NewValueFromPlainObject(processObj))
}

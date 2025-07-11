package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

type RegExpInitializer struct{}

func (r *RegExpInitializer) Name() string {
	return "RegExp"
}

func (r *RegExpInitializer) Priority() int {
	return PriorityRegExp // After basic types
}

func (r *RegExpInitializer) InitTypes(ctx *TypeContext) error {
	// Create RegExp.prototype type with methods
	regexpProtoType := types.NewObjectType().
		WithProperty("source", types.String).
		WithProperty("flags", types.String).
		WithProperty("global", types.Boolean).
		WithProperty("ignoreCase", types.Boolean).
		WithProperty("multiline", types.Boolean).
		WithProperty("dotAll", types.Boolean).
		WithProperty("lastIndex", types.Number).
		WithProperty("test", types.NewSimpleFunction([]types.Type{types.String}, types.Boolean)).
		WithProperty("exec", types.NewSimpleFunction([]types.Type{types.String}, types.NewUnionType(types.Null, &types.ArrayType{ElementType: types.String}))).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Register RegExp primitive prototype
	ctx.SetPrimitivePrototype("RegExp", regexpProtoType)

	// Create RegExp constructor type
	regexpCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, types.RegExp).                                                     // RegExp() -> RegExp
		WithSimpleCallSignature([]types.Type{types.String}, types.RegExp).                                        // RegExp(pattern) -> RegExp
		WithSimpleCallSignature([]types.Type{types.String, types.String}, types.RegExp).                          // RegExp(pattern, flags) -> RegExp
		WithSimpleCallSignature([]types.Type{types.RegExp}, types.RegExp).                                        // RegExp(regexObj) -> RegExp
		WithSimpleConstructSignature([]types.Type{}, types.RegExp).                                               // new RegExp() -> RegExp
		WithSimpleConstructSignature([]types.Type{types.String}, types.RegExp).                                   // new RegExp(pattern) -> RegExp
		WithSimpleConstructSignature([]types.Type{types.String, types.String}, types.RegExp).                     // new RegExp(pattern, flags) -> RegExp
		WithSimpleConstructSignature([]types.Type{types.RegExp}, types.RegExp).                                   // new RegExp(regexObj) -> RegExp
		WithProperty("prototype", regexpProtoType)

	return ctx.DefineGlobal("RegExp", regexpCtorType)
}

func (r *RegExpInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create RegExp.prototype inheriting from Object.prototype  
	regexpProto := vm.NewObject(objectProto).AsPlainObject()

	// Add RegExp prototype methods
	regexpProto.SetOwn("test", vm.NewNativeFunction(1, false, "test", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, nil
		}
		regex := thisRegex.AsRegExpObject()
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		str := args[0].ToString()
		matched := regex.GetCompiledRegex().MatchString(str)
		return vm.BooleanValue(matched), nil
	}))

	regexpProto.SetOwn("exec", vm.NewNativeFunction(1, false, "exec", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, nil
		}
		regex := thisRegex.AsRegExpObject()
		if len(args) == 0 {
			return vm.Null, nil
		}
		str := args[0].ToString()
		compiledRegex := regex.GetCompiledRegex()

		var matches []string
		if regex.IsGlobal() {
			// Global regex: use lastIndex for stateful matching
			remainder := str[regex.GetLastIndex():]
			if loc := compiledRegex.FindStringSubmatchIndex(remainder); loc != nil {
				matches = compiledRegex.FindStringSubmatch(remainder)
				// Update lastIndex
				regex.SetLastIndex(regex.GetLastIndex() + loc[1])
			}
		} else {
			// Non-global: find first match
			matches = compiledRegex.FindStringSubmatch(str)
		}

		if matches == nil {
			if regex.IsGlobal() {
				regex.SetLastIndex(0) // Reset lastIndex on failure
			}
			return vm.Null, nil
		}

		// Create result array with matches
		result := vm.NewArray()
		arr := result.AsArray()
		for _, match := range matches {
			arr.Append(vm.NewString(match))
		}
		return result, nil
	}))

	regexpProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, nil
		}
		regex := thisRegex.AsRegExpObject()
		result := "/" + regex.GetSource() + "/" + regex.GetFlags()
		return vm.NewString(result), nil
	}))

	// Create RegExp constructor function with properties
	regexpCtor := vm.NewNativeFunctionWithProps(-1, true, "RegExp", func(args []vm.Value) (vm.Value, error) {
		// Constructor logic
		if len(args) == 0 {
			// new RegExp() - empty pattern
			result, _ := vm.NewRegExp("(?:)", "")
			return result, nil
		}

		if len(args) == 1 {
			arg := args[0]
			if arg.IsRegExp() {
				// Copy constructor: new RegExp(regexObj)
				existing := arg.AsRegExpObject()
				result, _ := vm.NewRegExp(existing.GetSource(), existing.GetFlags())
				return result, nil
			} else {
				// new RegExp(pattern) - convert to string and use empty flags
				pattern := arg.ToString()
				result, _ := vm.NewRegExp(pattern, "")
				return result, nil
			}
		}

		// new RegExp(pattern, flags)
		pattern := args[0].ToString()
		flags := args[1].ToString()
		result, _ := vm.NewRegExp(pattern, flags)
		return result, nil
	})

	// Set up prototype relationship
	regexpCtor.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(regexpProto))

	// Register RegExp prototype in VM
	vmInstance.RegExpPrototype = vm.NewValueFromPlainObject(regexpProto)

	return ctx.DefineGlobal("RegExp", regexpCtor)
}
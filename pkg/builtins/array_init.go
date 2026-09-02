package builtins

import (
	"errors"
	"fmt"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// toIntegerOrInfinityWithVM converts a value to an integer using proper ECMAScript semantics.
// It calls ToPrimitive if needed and propagates exceptions from valueOf/toString.
// Returns (result, nil) on success, (0, ErrVMUnwinding) if ToPrimitive threw, or a TypeError for Symbols.
func toIntegerOrInfinityWithVM(vmInstance *vm.VM, val vm.Value) (int, error) {
	// Check for Symbol first - cannot convert Symbol to number
	if val.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}

	// For primitives, no ToPrimitive call is needed
	if !val.IsObject() && !val.IsCallable() {
		return int(val.ToFloat()), nil
	}

	// For objects, call ToPrimitive which may invoke valueOf/toString
	vmInstance.EnterHelperCall()
	primVal := vmInstance.ToPrimitive(val, "number")
	vmInstance.ExitHelperCall()

	// Check if ToPrimitive threw an exception
	if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
		return 0, ErrVMUnwinding
	}

	// Check if ToPrimitive returned a Symbol (from [Symbol.toPrimitive])
	if primVal.Type() == vm.TypeSymbol {
		return 0, vmInstance.NewTypeError("Cannot convert a Symbol value to a number")
	}

	return int(primVal.ToFloat()), nil
}

type ArrayInitializer struct{}

func (a *ArrayInitializer) Name() string {
	return "Array"
}

func (a *ArrayInitializer) Priority() int {
	return PriorityArray // 3 - After Object (0), Function (1), Iterator (2)
}

func (a *ArrayInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for array methods
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}
	tArrayType := &types.ArrayType{ElementType: tType}

	// Create Array.prototype type with selective generic methods
	arrayProtoType := types.NewObjectType().
		WithProperty("length", types.Number).
		// Keep mutation methods non-generic for flexibility
		WithVariadicProperty("push", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}).
		WithProperty("pop", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithProperty("shift", types.NewSimpleFunction([]types.Type{}, types.Any)).
		WithVariadicProperty("unshift", []types.Type{}, types.Number, &types.ArrayType{ElementType: types.Any}).
		WithProperty("slice", a.createGenericMethod("slice", tParam,
			types.NewOptionalFunction([]types.Type{types.Number, types.Number}, tArrayType, []bool{true, true}))).
		// Keep concat non-generic for flexibility with different array types
		WithVariadicProperty("concat", []types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}).
		WithProperty("join", types.NewOptionalFunction([]types.Type{types.String}, types.String, []bool{true})).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("reverse", a.createGenericMethod("reverse", tParam,
			types.NewSimpleFunction([]types.Type{}, tArrayType))).
		WithProperty("indexOf", a.createGenericMethod("indexOf", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number}, types.Number, []bool{false, true}))).
		WithProperty("lastIndexOf", a.createGenericMethod("lastIndexOf", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number}, types.Number, []bool{false, true}))).
		WithProperty("includes", a.createGenericMethod("includes", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number}, types.Boolean, []bool{false, true}))).
		// Make callback-based methods generic (these are the important ones!)
		WithProperty("find", a.createGenericMethod("find", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.NewUnionType(tType, types.Undefined)))).
		WithProperty("findIndex", a.createGenericMethod("findIndex", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Number))).
		WithProperty("filter", a.createGenericMethod("filter", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				tArrayType))).
		WithProperty("map", a.createGenericMapMethod(tParam)).
		WithProperty("forEach", a.createGenericMethod("forEach", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Undefined, []bool{false, true, true})},
				types.Undefined))).
		WithProperty("every", a.createGenericMethod("every", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Boolean))).
		WithProperty("some", a.createGenericMethod("some", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Boolean))).
		// Keep reduce non-generic for now since it's complex
		WithProperty("reduce", types.NewOptionalFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any), types.Any}, types.Any, []bool{false, true})).
		WithProperty("reduceRight", types.NewOptionalFunction([]types.Type{types.NewSimpleFunction([]types.Type{types.Any, types.Any, types.Number, &types.ArrayType{ElementType: types.Any}}, types.Any), types.Any}, types.Any, []bool{false, true})).
		// splice: (start: number, deleteCount?: number, ...items: T[]) => T[]
		WithProperty("splice", a.createGenericMethod("splice", tParam,
			&types.ObjectType{
				CallSignatures: []*types.Signature{
					types.SigOptional([]types.Type{types.Number, types.Number}, tArrayType, []bool{false, true}),
					types.SigVariadic([]types.Type{types.Number, types.Number}, tArrayType, tType),
				},
			})).
		// sort: (comparefn?: (a: T, b: T) => number) => T[]
		WithProperty("sort", a.createGenericMethod("sort", tParam,
			types.NewOptionalFunction([]types.Type{
				types.NewSimpleFunction([]types.Type{tType, tType}, types.Number)},
				tArrayType, []bool{true}))).
		// at: (index: number) => T | undefined
		WithProperty("at", a.createGenericMethod("at", tParam,
			types.NewSimpleFunction([]types.Type{types.Number}, types.NewUnionType(tType, types.Undefined)))).
		// findLast: (predicate: (value: T, index?: number, array?: T[]) => boolean) => T | undefined
		WithProperty("findLast", a.createGenericMethod("findLast", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.NewUnionType(tType, types.Undefined)))).
		// findLastIndex: (predicate: (value: T, index?: number, array?: T[]) => boolean) => number
		WithProperty("findLastIndex", a.createGenericMethod("findLastIndex", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Boolean, []bool{false, true, true})},
				types.Number))).
		// copyWithin: (target: number, start?: number, end?: number) => T[]
		WithProperty("copyWithin", a.createGenericMethod("copyWithin", tParam,
			types.NewOptionalFunction([]types.Type{types.Number, types.Number, types.Number}, tArrayType, []bool{false, true, true}))).
		// fill: (value: T, start?: number, end?: number) => T[]
		WithProperty("fill", a.createGenericMethod("fill", tParam,
			types.NewOptionalFunction([]types.Type{tType, types.Number, types.Number}, tArrayType, []bool{false, true, true}))).
		// flat: (depth?: number) => any[]
		WithProperty("flat", types.NewOptionalFunction([]types.Type{types.Number}, &types.ArrayType{ElementType: types.Any}, []bool{true})).
		// flatMap: (callback: (value: T, index: number, array: T[]) => any) => any[]
		WithProperty("flatMap", a.createGenericMethod("flatMap", tParam,
			types.NewSimpleFunction([]types.Type{
				types.NewOptionalFunction([]types.Type{tType, types.Number, tArrayType}, types.Any, []bool{false, true, true})},
				&types.ArrayType{ElementType: types.Any}))).
		// toSorted: (comparefn?: (a: T, b: T) => number) => T[]
		WithProperty("toSorted", a.createGenericMethod("toSorted", tParam,
			types.NewOptionalFunction([]types.Type{
				types.NewSimpleFunction([]types.Type{tType, tType}, types.Number)},
				tArrayType, []bool{true}))).
		// toSpliced: (start: number, deleteCount?: number, ...items: T[]) => T[]
		WithProperty("toSpliced", a.createGenericMethod("toSpliced", tParam,
			&types.ObjectType{
				CallSignatures: []*types.Signature{
					types.SigOptional([]types.Type{types.Number, types.Number}, tArrayType, []bool{false, true}),
					types.SigVariadic([]types.Type{types.Number, types.Number}, tArrayType, tType),
				},
			})).
		// toReversed: () => T[]
		WithProperty("toReversed", a.createGenericMethod("toReversed", tParam,
			types.NewSimpleFunction([]types.Type{}, tArrayType))).
		// with: (index: number, value: T) => T[]
		WithProperty("with", a.createGenericMethod("with", tParam,
			types.NewSimpleFunction([]types.Type{types.Number, tType}, tArrayType)))

	// Add Symbol.iterator to array prototype type to make arrays iterable
	// Get the Iterator<T> generic type if available (use internal name)
	if iteratorType, found := ctx.GetType("__IteratorGeneric__"); found {
		if iteratorGeneric, ok := iteratorType.(*types.GenericType); ok {
			// Create Iterator<T> type for arrays
			iteratorOfT := &types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tType},
			}
			// Add [Symbol.iterator](): Iterator<T> method (computed symbol key in types)
			arrayProtoType = arrayProtoType.WithProperty("__COMPUTED_PROPERTY__",
				a.createGenericMethod("[Symbol.iterator]", tParam,
					types.NewSimpleFunction([]types.Type{}, iteratorOfT)))

			// Add values(): Iterator<T> method (same as [Symbol.iterator])
			arrayProtoType = arrayProtoType.WithProperty("values",
				a.createGenericMethod("values", tParam,
					types.NewSimpleFunction([]types.Type{}, iteratorOfT)))

			// Add keys(): Iterator<number> method
			iteratorOfNumber := &types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{types.Number},
			}
			arrayProtoType = arrayProtoType.WithProperty("keys",
				types.NewSimpleFunction([]types.Type{}, iteratorOfNumber))

			// Add entries(): Iterator<[number, T]> method
			tupleType := &types.TupleType{ElementTypes: []types.Type{types.Number, tType}}
			iteratorOfEntries := &types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{tupleType},
			}
			arrayProtoType = arrayProtoType.WithProperty("entries",
				a.createGenericMethod("entries", tParam,
					types.NewSimpleFunction([]types.Type{}, iteratorOfEntries)))
		}
	}

	// Register array primitive prototype
	ctx.SetPrimitivePrototype("array", arrayProtoType)

	fromAsyncReturnType := &types.InstantiatedType{
		Generic:       types.PromiseGeneric,
		TypeArguments: []types.Type{&types.ArrayType{ElementType: types.Any}},
	}

	// Create Array constructor type
	arrayCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, &types.ArrayType{ElementType: types.Any}).                                             // Array() -> array
		WithSimpleCallSignature([]types.Type{types.Number}, &types.ArrayType{ElementType: types.Any}).                                 // Array(length) -> array
		WithVariadicCallSignature([]types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}). // Array(...elements) -> array
		WithProperty("isArray", types.NewSimpleFunction([]types.Type{types.Any}, &types.TypePredicateType{ParameterName: "arg", Type: &types.ArrayType{ElementType: types.Any}})).
		WithProperty("from", types.NewOptionalFunction([]types.Type{types.Any, types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Any)}, &types.ArrayType{ElementType: types.Any}, []bool{false, true})).
		WithProperty("fromAsync", types.NewOptionalFunction([]types.Type{types.Any, types.NewSimpleFunction([]types.Type{types.Any, types.Number}, types.Any), types.Any}, fromAsyncReturnType, []bool{false, true, true})).
		WithVariadicProperty("of", []types.Type{}, &types.ArrayType{ElementType: types.Any}, &types.ArrayType{ElementType: types.Any}).
		WithProperty("prototype", arrayProtoType)

	// Define Array constructor in global environment
	return ctx.DefineGlobal("Array", arrayCtorType)
}

func (a *ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Array.prototype inheriting from Object.prototype
	arrayProto := vm.NewObject(objectProto).AsPlainObject()

	// Set Array.prototype.length = 0 per ECMAScript spec
	arrayProto.SetOwnNonEnumerable("length", vm.NumberValue(0))

	// Add Array prototype methods
	arrayProto.SetOwnNonEnumerable("push", vm.NewNativeFunction(1, true, "push", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		// No TypeArray fast path (see pop's identical comment above for
		// why): the old one appended directly, bypassing
		// Set(..., "length", ..., true)'s required TypeError on a
		// frozen/non-writable-length array (the no-argument case,
		// push() with zero elements to write still performs this step).
		// Known gap this doesn't close: writing each new index still
		// goes through arrayLikeSet's plain arr.Set(i, val), not a full
		// [[Set]] that would walk Array.prototype for an inherited setter
		// - so a setter defined on Array.prototype[<next index>] that
		// freezes the array mid-push (the same trick pop's
		// set-length-array-is-frozen.js test uses, via a getter) isn't
		// observed here; push's own version of that test needs that walk.
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		for _, arg := range args {
			if err := arrayLikeSet(vmInstance, thisVal, length, arg); err != nil {
				return vm.Undefined, err
			}
			length++
		}
		if err := arrayLikeSetLength(vmInstance, thisVal, length); err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(length)), nil
	}))

	arrayProto.SetOwnNonEnumerable("pop", vm.NewNativeFunction(0, false, "pop", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		// No TypeArray fast path here (unlike several sibling methods
		// below): the generic arrayLike* helpers ARE the fast path for a
		// real Array (arrayLikeLength/Get/Delete/SetLength each resolve to
		// a couple of ArrayObject method calls, no extra allocation), and
		// skipping them like the old fast path here did meant bypassing
		// DeletePropertyOrThrow and Set(..., "length", ..., true)'s
		// required TypeError on a frozen/non-writable-length array - a
		// gap Test262's set-length-*-is-frozen.js /
		// set-length-*-length-is-non-writable.js cover, including one
		// (set-length-array-is-frozen.js) that only throws because a
		// getter on Array.prototype[0] freezes the array *during* the Get
		// step, before the length write - arrayLikeGet's prototype-chain
		// fallback (for the hole this creates) is what makes that
		// ordering observable at all.
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if length == 0 {
			return vm.Undefined, arrayLikeSetLength(vmInstance, thisVal, 0)
		}
		newLen := length - 1
		element, _, err := arrayLikeGet(vmInstance, thisVal, newLen)
		if err != nil {
			return vm.Undefined, err
		}
		if err := arrayLikeDelete(vmInstance, thisVal, newLen); err != nil {
			return vm.Undefined, err
		}
		if err := arrayLikeSetLength(vmInstance, thisVal, newLen); err != nil {
			return vm.Undefined, err
		}
		return element, nil
	}))

	arrayProto.SetOwnNonEnumerable("shift", vm.NewNativeFunction(0, false, "shift", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		// No TypeArray fast path (see pop's identical comment just above
		// for why): the old one rebuilt elements directly, bypassing
		// DeletePropertyOrThrow/Set(..., "length", ..., true)'s required
		// TypeError on a frozen/non-writable-length array.
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if length == 0 {
			return vm.Undefined, arrayLikeSetLength(vmInstance, thisVal, 0)
		}
		first, _, err := arrayLikeGet(vmInstance, thisVal, 0)
		if err != nil {
			return vm.Undefined, err
		}
		for k := 1; k < length; k++ {
			from, to := k, k-1
			v, exists, err := arrayLikeGet(vmInstance, thisVal, from)
			if err != nil {
				return vm.Undefined, err
			}
			if exists {
				if err := arrayLikeSet(vmInstance, thisVal, to, v); err != nil {
					return vm.Undefined, err
				}
			} else if err := arrayLikeDelete(vmInstance, thisVal, to); err != nil {
				return vm.Undefined, err
			}
		}
		if err := arrayLikeDelete(vmInstance, thisVal, length-1); err != nil {
			return vm.Undefined, err
		}
		if err := arrayLikeSetLength(vmInstance, thisVal, length-1); err != nil {
			return vm.Undefined, err
		}
		return first, nil
	}))

	arrayProto.SetOwnNonEnumerable("unshift", vm.NewNativeFunction(1, true, "unshift", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		// No TypeArray fast path (see pop's identical comment above for
		// why): the old one rebuilt elements directly, bypassing
		// Set(..., "length", ..., true)'s required TypeError on a
		// frozen/non-writable-length array (the zero-argument case still
		// performs this step).
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		argCount := len(args)
		if argCount > 0 {
			// Per spec step 4a: throw TypeError before shifting anything if
			// the new length would exceed 2^53-1 - length is already
			// clamped to that bound by ToLength, but adding argCount on top
			// of an already-maxed length can legitimately push past it, and
			// without this check the shift loop below would attempt on the
			// order of 2^53 iterations even when (as here) every access is
			// individually cheap.
			if length+argCount > maxSafeInteger {
				return vm.Undefined, vmInstance.NewTypeError("Invalid array length")
			}
			for k := length; k > 0; k-- {
				from, to := k-1, k+argCount-1
				v, exists, err := arrayLikeGet(vmInstance, thisVal, from)
				if err != nil {
					return vm.Undefined, err
				}
				if exists {
					if err := arrayLikeSet(vmInstance, thisVal, to, v); err != nil {
						return vm.Undefined, err
					}
				} else if err := arrayLikeDelete(vmInstance, thisVal, to); err != nil {
					return vm.Undefined, err
				}
			}
			for j, arg := range args {
				if err := arrayLikeSet(vmInstance, thisVal, j, arg); err != nil {
					return vm.Undefined, err
				}
			}
		}
		newLen := length + argCount
		if err := arrayLikeSetLength(vmInstance, thisVal, newLen); err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(newLen)), nil
	}))

	arrayProto.SetOwnNonEnumerable("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		start := 0
		if len(args) >= 1 && !args[0].IsUndefined() {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if start < 0 {
				start = length + start
				if start < 0 {
					start = 0
				}
			} else if start > length {
				start = length
			}
		}
		end := length
		if len(args) >= 2 && !args[1].IsUndefined() {
			var err error
			end, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if end < 0 {
				end = length + end
				if end < 0 {
					end = 0
				}
			} else if end > length {
				end = length
			}
		}
		if start >= end {
			return vm.NewArray(), nil
		}
		// ArraySpeciesCreate(O, count) throws before any copying if count
		// exceeds the max valid Array length - critically before the make()
		// below, which would otherwise try to allocate count elements (up
		// to 2^53-1 for an arbitrary array-like's declared length).
		count := end - start
		if err := checkArrayCreateLength(vmInstance, count); err != nil {
			return vm.Undefined, err
		}
		elements := make([]vm.Value, count)
		for i := start; i < end; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			elements[i-start] = v
		}
		return vm.NewArrayWithArgs(elements), nil
	}))

	arrayProto.SetOwnNonEnumerable("splice", vm.NewNativeFunction(2, true, "splice", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		start := 0
		if len(args) >= 1 {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if start < 0 {
				start = length + start
				if start < 0 {
					start = 0
				}
			} else if start > length {
				start = length
			}
		}
		deleteCount := length - start
		if len(args) >= 2 {
			var err error
			deleteCount, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if deleteCount < 0 {
				deleteCount = 0
			} else if deleteCount > length-start {
				deleteCount = length - start
			}
		}
		var itemsToInsert []vm.Value
		if len(args) >= 2 {
			itemsToInsert = args[2:]
		}
		itemCount := len(itemsToInsert)

		// ArraySpeciesCreate(O, actualDeleteCount) throws before any copying
		// if actualDeleteCount exceeds the max valid Array length (see
		// checkArrayCreateLength) - actualDeleteCount is bounded by
		// length-start, which can itself be huge for an arbitrary
		// array-like's declared length.
		if err := checkArrayCreateLength(vmInstance, deleteCount); err != nil {
			return vm.Undefined, err
		}

		// Per spec step 8: if the resulting length would exceed 2^53-1,
		// throw a TypeError before doing any shifting - length is already
		// clamped to that bound by ToLength, but adding itemCount on top of
		// an already-maxed length can legitimately push newLength one past
		// it. Without this check, the shift loops below would attempt on
		// the order of 2^53 iterations.
		newLength := length - deleteCount + itemCount
		if newLength > maxSafeInteger {
			return vm.Undefined, vmInstance.NewTypeError("Invalid array length")
		}

		// Create array with deleted elements
		deleted := vm.NewArray()
		deletedArr := deleted.AsArray()
		for i := 0; i < deleteCount; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, start+i)
			if err != nil {
				return vm.Undefined, err
			}
			deletedArr.Append(v)
		}

		if itemCount < deleteCount {
			// Shift the tail left to close the gap
			for i := start; i < length-deleteCount; i++ {
				from, to := i+deleteCount, i+itemCount
				v, exists, err := arrayLikeGet(vmInstance, thisVal, from)
				if err != nil {
					return vm.Undefined, err
				}
				if exists {
					if err := arrayLikeSet(vmInstance, thisVal, to, v); err != nil {
						return vm.Undefined, err
					}
				} else if err := arrayLikeDelete(vmInstance, thisVal, to); err != nil {
					return vm.Undefined, err
				}
			}
			for i := length; i > newLength; i-- {
				if err := arrayLikeDelete(vmInstance, thisVal, i-1); err != nil {
					return vm.Undefined, err
				}
			}
		} else if itemCount > deleteCount {
			// Shift the tail right to make room
			for i := length - deleteCount; i > start; i-- {
				from, to := i+deleteCount-1, i+itemCount-1
				v, exists, err := arrayLikeGet(vmInstance, thisVal, from)
				if err != nil {
					return vm.Undefined, err
				}
				if exists {
					if err := arrayLikeSet(vmInstance, thisVal, to, v); err != nil {
						return vm.Undefined, err
					}
				} else if err := arrayLikeDelete(vmInstance, thisVal, to); err != nil {
					return vm.Undefined, err
				}
			}
		}
		for i, item := range itemsToInsert {
			if err := arrayLikeSet(vmInstance, thisVal, start+i, item); err != nil {
				return vm.Undefined, err
			}
		}
		if err := arrayLikeSetLength(vmInstance, thisVal, newLength); err != nil {
			return vm.Undefined, err
		}
		return deleted, nil
	}))

	arrayProto.SetOwnNonEnumerable("concat", vm.NewNativeFunction(1, true, "concat", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		result := vm.NewArray()
		resultArr := result.AsArray()

		// n tracks the total element count appended so far across every
		// item, so the "would exceed 2^53-1" check below applies to the
		// combined result, not just one item - per spec step 5.c.iii, this
		// must be checked (and throw TypeError) *before* spreading a given
		// item's elements, since a spreadable array-like can freely declare
		// any "length" up to 2^53-1 and a bare loop over it would otherwise
		// take on the order of 2^53 iterations.
		n := 0
		appendItem := func(item vm.Value) error {
			spreadable, err := isConcatSpreadable(vmInstance, item)
			if err != nil {
				return err
			}
			if !spreadable {
				if n+1 > maxSafeInteger {
					return vmInstance.NewTypeError("Invalid array length")
				}
				resultArr.Append(item)
				n++
				return nil
			}
			length, err := arrayLikeLength(vmInstance, item)
			if err != nil {
				return err
			}
			if n+length > maxSafeInteger {
				return vmInstance.NewTypeError("Invalid array length")
			}
			for i := 0; i < length; i++ {
				v, _, err := arrayLikeGet(vmInstance, item, i)
				if err != nil {
					return err
				}
				resultArr.Append(v)
			}
			n += length
			return nil
		}

		if err := appendItem(thisVal); err != nil {
			return vm.Undefined, err
		}
		for _, arg := range args {
			if err := appendItem(arg); err != nil {
				return vm.Undefined, err
			}
		}
		return result, nil
	}))

	arrayProto.SetOwnNonEnumerable("join", vm.NewNativeFunction(1, false, "join", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		separator := ","
		if len(args) >= 1 {
			separator = args[0].ToString()
		}
		if length == 0 {
			return vm.NewString(""), nil
		}
		first, _, err := arrayLikeGet(vmInstance, thisVal, 0)
		if err != nil {
			return vm.Undefined, err
		}
		result := first.ToString()
		for i := 1; i < length; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			result += separator + v.ToString()
		}
		return vm.NewString(result), nil
	}))

	arrayProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		// Per ECMAScript spec, Array.prototype.toString is equivalent to calling join() with no arguments
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if length == 0 {
			return vm.NewString(""), nil
		}
		first, _, err := arrayLikeGet(vmInstance, thisVal, 0)
		if err != nil {
			return vm.Undefined, err
		}
		result := first.ToString()
		for i := 1; i < length; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			result += "," + v.ToString()
		}
		return vm.NewString(result), nil
	}))

	arrayProto.SetOwnNonEnumerable("reverse", vm.NewNativeFunction(0, false, "reverse", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		if thisVal.Type() == vm.TypeArray {
			thisArray := thisVal.AsArray()
			length := thisArray.Length()
			for i := 0; i < length/2; i++ {
				j := length - 1 - i
				left := thisArray.Get(i)
				right := thisArray.Get(j)
				thisArray.Set(i, right)
				thisArray.Set(j, left)
			}
			return thisVal, nil
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		for i := 0; i < length/2; i++ {
			j := length - 1 - i
			left, leftExists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			right, rightExists, err := arrayLikeGet(vmInstance, thisVal, j)
			if err != nil {
				return vm.Undefined, err
			}
			if rightExists {
				if err := arrayLikeSet(vmInstance, thisVal, i, right); err != nil {
					return vm.Undefined, err
				}
			} else if err := arrayLikeDelete(vmInstance, thisVal, i); err != nil {
				return vm.Undefined, err
			}
			if leftExists {
				if err := arrayLikeSet(vmInstance, thisVal, j, left); err != nil {
					return vm.Undefined, err
				}
			} else if err := arrayLikeDelete(vmInstance, thisVal, j); err != nil {
				return vm.Undefined, err
			}
		}
		return thisVal, nil
	}))

	arrayProto.SetOwnNonEnumerable("sort", vm.NewNativeFunction(1, false, "sort", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if length <= 1 {
			return thisVal, nil
		}
		// Extract elements to slice
		elements := make([]vm.Value, length)
		for i := 0; i < length; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			elements[i] = v
		}

		// Get comparator function if provided
		var compareFn vm.Value
		if len(args) > 0 && args[0].IsCallable() {
			compareFn = args[0]
		}

		// Simple bubble sort (not efficient but correct)
		for i := 0; i < length-1; i++ {
			for j := 0; j < length-i-1; j++ {
				var shouldSwap bool
				if compareFn.IsCallable() {
					// Use the comparator function
					result, err := vmInstance.CallArgs2(compareFn, vm.Undefined, elements[j], elements[j+1])
					if err != nil {
						return vm.Undefined, err
					}
					// Per ECMAScript: compareFn(a, b) > 0 means a should come after b
					shouldSwap = result.ToFloat() > 0
				} else {
					// Default: string comparison per ECMAScript spec
					shouldSwap = elements[j].ToString() > elements[j+1].ToString()
				}
				if shouldSwap {
					elements[j], elements[j+1] = elements[j+1], elements[j]
				}
			}
		}
		// Set sorted elements back
		if thisVal.Type() == vm.TypeArray {
			thisVal.AsArray().SetElements(elements)
		} else {
			for i, v := range elements {
				if err := arrayLikeSet(vmInstance, thisVal, i, v); err != nil {
					return vm.Undefined, err
				}
			}
		}
		return thisVal, nil
	}))

	arrayProto.SetOwnNonEnumerable("indexOf", vm.NewNativeFunction(1, false, "indexOf", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// If no argument, search for undefined
		var searchElement vm.Value
		if len(args) >= 1 {
			searchElement = args[0]
		} else {
			searchElement = vm.Undefined
		}
		if length == 0 {
			return vm.NumberValue(-1), nil
		}
		fromIndex := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if fromIndex < 0 {
				fromIndex = length + fromIndex
				if fromIndex < 0 {
					fromIndex = 0
				}
			}
		}
		// ECMAScript spec: indexOf uses Strict Equality (===), not SameValueZero
		// This means NaN !== NaN, so indexOf(NaN) should return -1
		for i := fromIndex; i < length; i++ {
			v, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			// Skip holes in sparse arrays/array-likes
			if !exists {
				continue
			}
			if v.StrictlyEquals(searchElement) {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	arrayProto.SetOwnNonEnumerable("lastIndexOf", vm.NewNativeFunction(1, false, "lastIndexOf", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// If no argument, search for undefined
		var searchElement vm.Value
		if len(args) >= 1 {
			searchElement = args[0]
		} else {
			searchElement = vm.Undefined
		}
		if length == 0 {
			return vm.NumberValue(-1), nil
		}
		fromIndex := length - 1
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if fromIndex < 0 {
				fromIndex = length + fromIndex
			} else if fromIndex >= length {
				fromIndex = length - 1
			}
		}
		// ECMAScript spec: lastIndexOf uses Strict Equality (===), not SameValueZero
		// This means NaN !== NaN, so lastIndexOf(NaN) should return -1
		for i := fromIndex; i >= 0; i-- {
			v, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			// Skip holes in sparse arrays/array-likes
			if !exists {
				continue
			}
			if v.StrictlyEquals(searchElement) {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	arrayProto.SetOwnNonEnumerable("includes", vm.NewNativeFunction(1, false, "includes", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// If no argument, search for undefined
		var searchElement vm.Value
		if len(args) >= 1 {
			searchElement = args[0]
		} else {
			searchElement = vm.Undefined
		}
		if length == 0 {
			return vm.BooleanValue(false), nil
		}
		fromIndex := 0
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			var err error
			fromIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if fromIndex < 0 {
				fromIndex = length + fromIndex
				if fromIndex < 0 {
					fromIndex = 0
				}
			}
		}
		// ECMAScript spec: includes uses SameValueZero, so NaN === NaN (Is() is correct)
		// Note: includes DOES check holes and finds undefined in them (unlike indexOf)
		for i := fromIndex; i < length; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			if v.Is(searchElement) {
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	}))

	arrayProto.SetOwnNonEnumerable("find", vm.NewNativeFunction(1, false, "find", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.find called on null or undefined")
		}
		if len(args) < 1 {
			return vm.Undefined, nil
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("predicate is not a function")
		}
		// Get thisArg (second argument to find)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}
		for i := 0; i < length; i++ {
			element, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			result, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.Undefined, err
			}
			if result.IsTruthy() {
				return element, nil
			}
		}
		return vm.Undefined, nil
	}))

	arrayProto.SetOwnNonEnumerable("findIndex", vm.NewNativeFunction(1, false, "findIndex", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.findIndex called on null or undefined")
		}
		if len(args) < 1 {
			return vm.NumberValue(-1), nil
		}
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		callback := args[0]
		if !callback.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("predicate is not a function")
		}
		// Get thisArg (second argument to findIndex)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}
		for i := 0; i < length; i++ {
			element, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			result, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.NumberValue(-1), err
			}
			if result.IsTruthy() {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	arrayProto.SetOwnNonEnumerable("filter", vm.NewNativeFunction(1, false, "filter", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.filter called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to filter)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		result := vm.NewArray()
		resultArr := result.AsArray()

		for i := 0; i < length; i++ {
			// Only call callback for indices that actually exist (sparse array support)
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.NewArray(), err
			}
			if !exists {
				continue
			}
			test, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.NewArray(), err
			}
			if test.IsTruthy() {
				resultArr.Append(element)
			}
		}
		return result, nil
	}))

	arrayProto.SetOwnNonEnumerable("map", vm.NewNativeFunction(1, false, "map", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.map called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to map)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// ArraySpeciesCreate/ArrayCreate(len) throws before any iteration if
		// len exceeds the max valid Array length - critically, this must
		// happen before the loop below, not after: len can be up to 2^53-1
		// (ToLength) for an arbitrary array-like receiver, and a bare loop
		// over a multi-billion length would otherwise hang the process.
		if err := checkArrayCreateLength(vmInstance, length); err != nil {
			return vm.Undefined, err
		}

		// Create result array with same length (for sparse array support)
		result := vm.NewArray()
		resultArr := result.AsArray()
		resultArr.SetLength(length)

		for i := 0; i < length; i++ {
			// Only call callback for indices that actually exist (sparse array support)
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			if !exists {
				continue
			}
			mappedValue, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.Undefined, err
			}
			resultArr.Set(i, mappedValue)
		}
		return result, nil
	}))

	arrayProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.forEach called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to forEach)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		for i := 0; i < length; i++ {
			// Only call callback for indices that actually exist (sparse array support)
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			if !exists {
				continue
			}
			if _, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal); err != nil {
				return vm.Undefined, err
			}
		}
		return vm.Undefined, nil
	}))

	arrayProto.SetOwnNonEnumerable("every", vm.NewNativeFunction(1, false, "every", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.every called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to every)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		for i := 0; i < length; i++ {
			// Only call callback for indices that actually exist (sparse array support)
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.BooleanValue(false), err
			}
			if !exists {
				continue
			}
			result, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.BooleanValue(false), err
			}
			if !result.IsTruthy() {
				return vm.BooleanValue(false), nil
			}
		}
		return vm.BooleanValue(true), nil
	}))

	arrayProto.SetOwnNonEnumerable("some", vm.NewNativeFunction(1, false, "some", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.some called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// Get thisArg (second argument to some)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		for i := 0; i < length; i++ {
			// Only call callback for indices that actually exist (sparse array support)
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.BooleanValue(false), err
			}
			if !exists {
				continue
			}
			result, err := vmInstance.CallArgs3(callback, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.BooleanValue(false), err
			}
			if result.IsTruthy() {
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	}))

	arrayProto.SetOwnNonEnumerable("reduce", vm.NewNativeFunction(1, false, "reduce", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.reduce called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// 5. If len = 0 and initialValue is not present, throw a TypeError exception.
		if length == 0 && len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reduce of empty array with no initial value")
		}

		var accumulator vm.Value
		startIndex := 0
		if len(args) >= 2 {
			accumulator = args[1]
		} else {
			v, _, err := arrayLikeGet(vmInstance, thisVal, 0)
			if err != nil {
				return vm.Undefined, err
			}
			accumulator = v
			startIndex = 1
		}

		for i := startIndex; i < length; i++ {
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			if !exists {
				continue
			}
			accumulator, err = vmInstance.CallArgs4(callback, vm.Undefined, accumulator, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.Undefined, err
			}
		}
		return accumulator, nil
	}))

	arrayProto.SetOwnNonEnumerable("reduceRight", vm.NewNativeFunction(1, false, "reduceRight", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.reduceRight called on null or undefined")
		}

		// 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
		var callback vm.Value
		if len(args) >= 1 {
			callback = args[0]
		} else {
			callback = vm.Undefined
		}
		if !callback.IsCallable() {
			callbackStr := "undefined"
			if callback.Type() != vm.TypeUndefined {
				callbackStr = callback.ToString()
			}
			return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
		}

		// 5. If len = 0 and initialValue is not present, throw a TypeError exception.
		if length == 0 && len(args) < 2 {
			return vm.Undefined, vmInstance.NewTypeError("Reduce of empty array with no initial value")
		}

		var accumulator vm.Value
		startIndex := length - 1
		if len(args) >= 2 {
			accumulator = args[1]
		} else {
			v, _, err := arrayLikeGet(vmInstance, thisVal, length-1)
			if err != nil {
				return vm.Undefined, err
			}
			accumulator = v
			startIndex = length - 2
		}

		for i := startIndex; i >= 0; i-- {
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			if !exists {
				continue
			}
			accumulator, err = vmInstance.CallArgs4(callback, vm.Undefined, accumulator, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.Undefined, err
			}
		}
		return accumulator, nil
	}))

	// Array.prototype.at - relative indexing access
	arrayProto.SetOwnNonEnumerable("at", vm.NewNativeFunction(1, false, "at", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.at called on null or undefined")
		}

		// Get length
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. Let relativeIndex be ? ToIntegerOrInfinity(index).
		var relativeIndex int
		if len(args) >= 1 {
			var err error
			relativeIndex, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}

		// 4. If relativeIndex ≥ 0, let k be relativeIndex. Else let k be len + relativeIndex.
		var k int
		if relativeIndex >= 0 {
			k = relativeIndex
		} else {
			k = length + relativeIndex
		}

		// 5. If k < 0 or k ≥ len, return undefined.
		if k < 0 || k >= length {
			return vm.Undefined, nil
		}

		// 6. Return ? Get(O, ! ToString(k)).
		v, _, err := arrayLikeGet(vmInstance, thisVal, k)
		if err != nil {
			return vm.Undefined, err
		}
		return v, nil
	}))

	// Array.prototype.findLast - find from end
	arrayProto.SetOwnNonEnumerable("findLast", vm.NewNativeFunction(1, false, "findLast", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.findLast called on null or undefined")
		}

		// 3. If IsCallable(predicate) is false, throw a TypeError exception.
		var predicate vm.Value
		if len(args) >= 1 {
			predicate = args[0]
		} else {
			predicate = vm.Undefined
		}
		if !predicate.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("predicate is not a function")
		}

		// Get thisArg (second argument to findLast)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// Get length and iterate backwards
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		for i := length - 1; i >= 0; i-- {
			element, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			result, err := vmInstance.CallArgs3(predicate, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.Undefined, err
			}
			if result.IsTruthy() {
				return element, nil
			}
		}
		return vm.Undefined, nil
	}))

	// Array.prototype.findLastIndex - find index from end
	arrayProto.SetOwnNonEnumerable("findLastIndex", vm.NewNativeFunction(1, false, "findLastIndex", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.findLastIndex called on null or undefined")
		}

		// 3. If IsCallable(predicate) is false, throw a TypeError exception.
		var predicate vm.Value
		if len(args) >= 1 {
			predicate = args[0]
		} else {
			predicate = vm.Undefined
		}
		if !predicate.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("predicate is not a function")
		}

		// Get thisArg (second argument to findLastIndex)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		// Get length and iterate backwards
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		for i := length - 1; i >= 0; i-- {
			element, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			result, err := vmInstance.CallArgs3(predicate, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.NumberValue(-1), err
			}
			if result.IsTruthy() {
				return vm.NumberValue(float64(i)), nil
			}
		}
		return vm.NumberValue(-1), nil
	}))

	// Array.prototype.copyWithin - copy sequence of elements within array
	arrayProto.SetOwnNonEnumerable("copyWithin", vm.NewNativeFunction(2, false, "copyWithin", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.copyWithin called on null or undefined")
		}

		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// 3. Let relativeTarget be ? ToIntegerOrInfinity(target).
		// Note: Must process arguments before any early returns, as they may throw
		var target int
		if len(args) >= 1 {
			var err error
			target, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Let exception propagate
				}
				return vm.Undefined, err // Return TypeError
			}
		}
		// 4. If relativeTarget < 0, let to be max(len + relativeTarget, 0); else let to be min(relativeTarget, len).
		var to int
		if target < 0 {
			to = length + target
			if to < 0 {
				to = 0
			}
		} else {
			to = target
			if to > length {
				to = length
			}
		}

		// 5. Let relativeStart be ? ToIntegerOrInfinity(start).
		var start int
		if len(args) >= 2 {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Let exception propagate
				}
				return vm.Undefined, err // Return TypeError
			}
		}
		// 6. If relativeStart < 0, let from be max(len + relativeStart, 0); else let from be min(relativeStart, len).
		var from int
		if start < 0 {
			from = length + start
			if from < 0 {
				from = 0
			}
		} else {
			from = start
			if from > length {
				from = length
			}
		}

		// 7. If end is undefined, let relativeEnd be len; else let relativeEnd be ? ToIntegerOrInfinity(end).
		var end int
		if len(args) >= 3 && args[2].Type() != vm.TypeUndefined {
			var err error
			end, err = toIntegerOrInfinityWithVM(vmInstance, args[2])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil // Let exception propagate
				}
				return vm.Undefined, err // Return TypeError
			}
		} else {
			end = length
		}
		// 8. If relativeEnd < 0, let final be max(len + relativeEnd, 0); else let final be min(relativeEnd, len).
		var final int
		if end < 0 {
			final = length + end
			if final < 0 {
				final = 0
			}
		} else {
			final = end
			if final > length {
				final = length
			}
		}

		// 9. Let count be min(final - from, len - to).
		count := final - from
		if length-to < count {
			count = length - to
		}

		// Copy the elements
		if count > 0 {
			copyOne := func(i int) error {
				v, exists, err := arrayLikeGet(vmInstance, thisVal, from+i)
				if err != nil {
					return err
				}
				if exists {
					return arrayLikeSet(vmInstance, thisVal, to+i, v)
				}
				return arrayLikeDelete(vmInstance, thisVal, to+i)
			}
			// Need to handle overlapping regions
			if from < to && to < from+count {
				// Copy backwards to avoid overwriting source
				for i := count - 1; i >= 0; i-- {
					if err := copyOne(i); err != nil {
						return vm.Undefined, err
					}
				}
			} else {
				// Copy forwards
				for i := 0; i < count; i++ {
					if err := copyOne(i); err != nil {
						return vm.Undefined, err
					}
				}
			}
		}

		return thisVal, nil
	}))

	// Array.prototype.fill - fill array with value
	arrayProto.SetOwnNonEnumerable("fill", vm.NewNativeFunction(1, false, "fill", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.fill called on null or undefined")
		}

		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}

		// Get value to fill
		value := vm.Undefined
		if len(args) >= 1 {
			value = args[0]
		}

		// Get start index (? ToIntegerOrInfinity)
		var start int
		if len(args) >= 2 {
			var err error
			start, err = toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start > length {
			start = length
		}

		// Get end index (? ToIntegerOrInfinity)
		end := length
		if len(args) >= 3 && args[2].Type() != vm.TypeUndefined {
			var err error
			end, err = toIntegerOrInfinityWithVM(vmInstance, args[2])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}
		if end < 0 {
			end = length + end
			if end < 0 {
				end = 0
			}
		} else if end > length {
			end = length
		}

		// Fill the array
		for i := start; i < end; i++ {
			if err := arrayLikeSet(vmInstance, thisVal, i, value); err != nil {
				return vm.Undefined, err
			}
		}

		return thisVal, nil
	}))

	// Array.prototype.flat - flatten nested arrays
	arrayProto.SetOwnNonEnumerable("flat", vm.NewNativeFunction(0, false, "flat", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.flat called on null or undefined")
		}

		// 3. Let depthNum be ? ToIntegerOrInfinity(depth) (default 1)
		depth := 1
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			var err error
			depth, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}

		result := vm.NewArray()
		resultArr := result.AsArray()

		// Helper function to flatten recursively
		var flattenInto func(source vm.Value, currentDepth int) error
		flattenInto = func(source vm.Value, currentDepth int) error {
			length, err := arrayLikeLength(vmInstance, source)
			if err != nil {
				return err
			}
			for i := 0; i < length; i++ {
				element, exists, err := arrayLikeGet(vmInstance, source, i)
				if err != nil {
					return err
				}
				if !exists {
					continue
				}
				if currentDepth > 0 && element.Type() == vm.TypeArray {
					if err := flattenInto(element, currentDepth-1); err != nil {
						return err
					}
				} else {
					resultArr.Append(element)
				}
			}
			return nil
		}

		if err := flattenInto(thisVal, depth); err != nil {
			return vm.Undefined, err
		}
		return result, nil
	}))

	// Array.prototype.flatMap - map then flatten by one level
	arrayProto.SetOwnNonEnumerable("flatMap", vm.NewNativeFunction(1, false, "flatMap", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.flatMap called on null or undefined")
		}

		// 3. If IsCallable(mapperFunction) is false, throw a TypeError exception.
		var mapper vm.Value
		if len(args) >= 1 {
			mapper = args[0]
		} else {
			mapper = vm.Undefined
		}
		if !mapper.IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("flatMap mapper is not a function")
		}

		// Get thisArg (second argument to flatMap)
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		result := vm.NewArray()
		resultArr := result.AsArray()

		// Get length and iterate
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		for i := 0; i < length; i++ {
			element, exists, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			if !exists {
				continue
			}
			mapped, err := vmInstance.CallArgs3(mapper, thisArg, element, vm.NumberValue(float64(i)), thisVal)
			if err != nil {
				return vm.Undefined, err
			}
			// Flatten by one level
			if mapped.Type() == vm.TypeArray {
				mappedArr := mapped.AsArray()
				for j := 0; j < mappedArr.Length(); j++ {
					resultArr.Append(mappedArr.Get(j))
				}
			} else {
				resultArr.Append(mapped)
			}
		}

		return result, nil
	}))

	// Array.prototype.toSorted - non-mutating sort
	arrayProto.SetOwnNonEnumerable("toSorted", vm.NewNativeFunction(1, false, "toSorted", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.toSorted called on null or undefined")
		}

		// Check comparator if provided
		var comparator vm.Value
		if len(args) >= 1 && args[0].Type() != vm.TypeUndefined {
			comparator = args[0]
			if !comparator.IsCallable() {
				return vm.Undefined, vmInstance.NewTypeError("comparator is not a function")
			}
		}

		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// ArrayCreate(len) throws before any iteration if len exceeds the
		// max valid Array length (see checkArrayCreateLength).
		if err := checkArrayCreateLength(vmInstance, length); err != nil {
			return vm.Undefined, err
		}

		// Create a copy
		result := vm.NewArray()
		resultArr := result.AsArray()

		for i := 0; i < length; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			resultArr.Append(v)
		}

		// Sort the copy using the same logic as sort
		n := resultArr.Length()
		if n <= 1 {
			return result, nil
		}

		// Simple insertion sort for stability
		for i := 1; i < n; i++ {
			key := resultArr.Get(i)
			j := i - 1
			for j >= 0 {
				cmp := 0
				if comparator.IsCallable() {
					res, err := vmInstance.CallArgs2(comparator, vm.Undefined, resultArr.Get(j), key)
					if err != nil {
						return vm.Undefined, err
					}
					cmp = int(res.ToFloat())
				} else {
					// Default string comparison
					a := resultArr.Get(j).ToString()
					b := key.ToString()
					if a > b {
						cmp = 1
					} else if a < b {
						cmp = -1
					}
				}
				if cmp <= 0 {
					break
				}
				resultArr.Set(j+1, resultArr.Get(j))
				j--
			}
			resultArr.Set(j+1, key)
		}

		return result, nil
	}))

	// Array.prototype.toSpliced - non-mutating splice
	arrayProto.SetOwnNonEnumerable("toSpliced", vm.NewNativeFunction(2, true, "toSpliced", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.toSpliced called on null or undefined")
		}

		// Get source length
		sourceLength, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// ArrayCreate(len) throws before any iteration if len exceeds the
		// max valid Array length (see checkArrayCreateLength).
		if err := checkArrayCreateLength(vmInstance, sourceLength); err != nil {
			return vm.Undefined, err
		}

		// Parse start argument (? ToIntegerOrInfinity)
		var actualStart int
		if len(args) >= 1 {
			start, err := toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if start < 0 {
				actualStart = sourceLength + start
				if actualStart < 0 {
					actualStart = 0
				}
			} else {
				actualStart = start
				if actualStart > sourceLength {
					actualStart = sourceLength
				}
			}
		}

		// Parse deleteCount argument (? ToIntegerOrInfinity)
		actualDeleteCount := 0
		if len(args) >= 2 {
			deleteCount, err := toIntegerOrInfinityWithVM(vmInstance, args[1])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
			if deleteCount < 0 {
				deleteCount = 0
			}
			actualDeleteCount = deleteCount
			if actualDeleteCount > sourceLength-actualStart {
				actualDeleteCount = sourceLength - actualStart
			}
		} else if len(args) >= 1 {
			// If start is present but deleteCount is not, delete to end
			actualDeleteCount = sourceLength - actualStart
		}

		// Items to insert
		insertItems := []vm.Value{}
		if len(args) > 2 {
			insertItems = args[2:]
		}

		// Create result array
		result := vm.NewArray()
		resultArr := result.AsArray()

		// Copy elements before start
		for i := 0; i < actualStart; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			resultArr.Append(v)
		}

		// Insert new elements
		for _, item := range insertItems {
			resultArr.Append(item)
		}

		// Copy remaining elements after deleted section
		for i := actualStart + actualDeleteCount; i < sourceLength; i++ {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			resultArr.Append(v)
		}

		return result, nil
	}))

	// Array.prototype.with - non-mutating element replacement
	arrayProto.SetOwnNonEnumerable("with", vm.NewNativeFunction(2, false, "with", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.with called on null or undefined")
		}

		// Get length
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// ArrayCreate(len) throws before any iteration if len exceeds the
		// max valid Array length (see checkArrayCreateLength).
		if err := checkArrayCreateLength(vmInstance, length); err != nil {
			return vm.Undefined, err
		}

		// 3. Let relativeIndex be ? ToIntegerOrInfinity(index).
		var index int
		if len(args) >= 1 {
			var err error
			index, err = toIntegerOrInfinityWithVM(vmInstance, args[0])
			if err != nil {
				if err == ErrVMUnwinding {
					return vm.Undefined, nil
				}
				return vm.Undefined, err
			}
		}

		// 4. If relativeIndex ≥ 0, let actualIndex be relativeIndex. Else let actualIndex be len + relativeIndex.
		actualIndex := index
		if index < 0 {
			actualIndex = length + index
		}

		// 5. If actualIndex ≥ len or actualIndex < 0, throw a RangeError exception.
		if actualIndex < 0 || actualIndex >= length {
			return vm.Undefined, vmInstance.NewRangeError(fmt.Sprintf("Invalid index: %d", index))
		}

		// Get value
		value := vm.Undefined
		if len(args) >= 2 {
			value = args[1]
		}

		// Create copy with replaced element
		result := vm.NewArray()
		resultArr := result.AsArray()

		for i := 0; i < length; i++ {
			if i == actualIndex {
				resultArr.Append(value)
				continue
			}
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			resultArr.Append(v)
		}

		return result, nil
	}))

	// Array.prototype.toReversed - non-mutating reverse
	arrayProto.SetOwnNonEnumerable("toReversed", vm.NewNativeFunction(0, false, "toReversed", func(args []vm.Value) (vm.Value, error) {
		// 1. Let O be ? ToObject(this value).
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Array.prototype.toReversed called on null or undefined")
		}

		// Get length
		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		// ArrayCreate(len) throws before any iteration if len exceeds the
		// max valid Array length (see checkArrayCreateLength).
		if err := checkArrayCreateLength(vmInstance, length); err != nil {
			return vm.Undefined, err
		}

		// Create reversed copy
		result := vm.NewArray()
		resultArr := result.AsArray()

		for i := length - 1; i >= 0; i-- {
			v, _, err := arrayLikeGet(vmInstance, thisVal, i)
			if err != nil {
				return vm.Undefined, err
			}
			resultArr.Append(v)
		}

		return result, nil
	}))

	// Create Array constructor (length=1 per spec, variadic for multiple args)
	ctorWithProps := vm.NewConstructorWithProps(1, true, "Array", func(args []vm.Value) (vm.Value, error) {
		// Per ECMAScript spec, the instance's [[Prototype]] is derived from
		// newTarget so that `class S extends Array {} new S()` produces an
		// instance with S.prototype rather than Array.prototype. For direct
		// `new Array()` newTarget IS the Array constructor, GetPrototypeFromConstructor
		// resolves to Array.prototype, and we leave the per-instance override
		// unset (default behavior). For subclass calls newTarget is the
		// subclass and we apply the override.
		var subclassProto vm.Value
		if nt := vmInstance.GetNewTarget(); nt.Type() != vm.TypeUndefined {
			if p, gpfcErr := vmInstance.GetPrototypeFromConstructor(nt, "%ArrayPrototype%"); gpfcErr == nil && p.IsObject() {
				if !p.StrictlyEquals(vmInstance.ArrayPrototype) {
					subclassProto = p
				}
			}
		}

		var result vm.Value
		switch {
		case len(args) == 0:
			result = vm.NewArray()
		case len(args) == 1 && args[0].IsNumber():
			// Per spec (23.1.1.1), the single-number-argument form requires
			// an exact array index: a non-negative integer that fits in the
			// ArrayCreate bound. Anything else - negative, fractional, or
			// >= 2^32 - throws RangeError rather than silently truncating
			// or (for a huge length) hanging the process on the eventual
			// element loop some other method would run over it.
			lengthFloat := args[0].ToFloat()
			length := int(lengthFloat)
			if lengthFloat < 0 || float64(length) != lengthFloat || length > maxArrayLength {
				return vm.Undefined, vmInstance.NewRangeError("Invalid array length")
			}
			result = vm.NewArray()
			result.AsArray().SetLength(length)
		default:
			result = vm.NewArrayWithArgs(args)
		}

		if subclassProto.IsObject() {
			result.AsArray().SetPrototype(subclassProto)
		}
		return result, nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", vm.NewValueFromPlainObject(arrayProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("isArray", vm.NewNativeFunction(1, false, "isArray", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		isArr, err := isArraySpec(vmInstance, args[0])
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(isArr), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("from", vm.NewNativeFunction(1, false, "from", func(args []vm.Value) (vm.Value, error) {
		// Step 1: Get items argument
		if len(args) < 1 {
			// No items provided - treat as undefined which throws TypeError
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		arrayLike := args[0]

		// Step 2: If items is null or undefined, throw TypeError
		if arrayLike.Type() == vm.TypeNull || arrayLike.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// Step 3: Get optional mapFn and thisArg
		var mapFn vm.Value = vm.Undefined
		var thisArg vm.Value = vm.Undefined
		if len(args) >= 2 && !args[1].IsUndefined() {
			// If mapFn is provided but not callable, throw TypeError
			if !args[1].IsCallable() {
				return vm.Undefined, vmInstance.NewTypeError("Array.from: mapFn must be a function")
			}
			mapFn = args[1]
		}
		if len(args) >= 3 {
			thisArg = args[2]
		}

		// If it's already an array, create a shallow copy
		if arrayLike.Type() == vm.TypeArray {
			sourceArray := arrayLike.AsArray()
			result := vm.NewArray()
			for i := 0; i < sourceArray.Length(); i++ {
				element := sourceArray.Get(i)
				// Apply mapping function if provided
				if mapFn.Type() != vm.TypeUndefined {
					vmInstance.EnterHelperCall()
					mapped, err := vmInstance.CallArgs2(mapFn, thisArg, element, vm.NumberValue(float64(i)))
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.NewArray(), nil
					}
					if err != nil {
						return vm.NewArray(), err
					}
					result.AsArray().Append(mapped)
				} else {
					result.AsArray().Append(element)
				}
			}
			return result, nil
		}

		// Check if the source is iterable (has Symbol.iterator)
		// This includes:
		// - Objects with Symbol.iterator
		// - Sets and Maps
		// - Primitive strings (natively iterable)
		// - Primitives whose prototype has Symbol.iterator (e.g., numbers with custom iterator)
		iteratorMethod := vm.Undefined
		hasIterator := false
		if arrayLike.Type() == vm.TypeSet || arrayLike.Type() == vm.TypeMap {
			if method, ok := vmInstance.GetSymbolProperty(arrayLike, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		} else if arrayLike.IsObject() {
			if method, ok := vmInstance.GetSymbolProperty(arrayLike, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		} else if arrayLike.Type() == vm.TypeString {
			// Primitive strings are natively iterable via String.prototype[Symbol.iterator]
			if method, ok := vmInstance.GetSymbolProperty(vmInstance.StringPrototype, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		} else if arrayLike.IsNumber() {
			// Check if Number.prototype has Symbol.iterator (e.g., user-defined)
			if method, ok := vmInstance.GetSymbolProperty(vmInstance.NumberPrototype, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		} else if arrayLike.Type() == vm.TypeBoolean {
			// Check if Boolean.prototype has Symbol.iterator (e.g., user-defined)
			if method, ok := vmInstance.GetSymbolProperty(vmInstance.BooleanPrototype, SymbolIterator); ok && method.IsCallable() {
				iteratorMethod = method
				hasIterator = true
			}
		}

		if hasIterator {
			// Use iterator protocol
			result := vm.NewArray()
			vmInstance.EnterHelperCall()
			iterator, err := vmInstance.Call(iteratorMethod, arrayLike, []vm.Value{})
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return vm.NewArray(), nil
			}
			if err != nil {
				return vm.NewArray(), err
			}

			nextMethod, err := vmInstance.GetProperty(iterator, "next")
			if err != nil || !nextMethod.IsCallable() {
				return vm.NewArray(), vmInstance.NewTypeError("iterator.next is not a function")
			}

			index := 0
			for {
				vmInstance.EnterHelperCall()
				iterResult, err := vmInstance.Call(nextMethod, iterator, []vm.Value{})
				vmInstance.ExitHelperCall()
				if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
					return vm.NewArray(), nil
				}
				if err != nil {
					return vm.NewArray(), err
				}

				done, _ := vmInstance.GetProperty(iterResult, "done")
				if done.IsTruthy() {
					break
				}

				value, _ := vmInstance.GetProperty(iterResult, "value")
				// Apply mapping function if provided
				if mapFn.Type() != vm.TypeUndefined {
					vmInstance.EnterHelperCall()
					mapped, mapErr := vmInstance.CallArgs2(mapFn, thisArg, value, vm.NumberValue(float64(index)))
					vmInstance.ExitHelperCall()
					if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
						return vm.NewArray(), nil
					}
					if mapErr != nil {
						return vm.NewArray(), mapErr
					}
					result.AsArray().Append(mapped)
				} else {
					result.AsArray().Append(value)
				}
				index++
			}
			return result, nil
		}

		// For array-like objects (has length property)
		if arrayLike.IsObject() {
			lengthVal, err := vmInstance.GetProperty(arrayLike, "length")
			if err == nil && lengthVal.IsNumber() {
				length := int(lengthVal.ToFloat())
				result := vm.NewArray()
				for i := 0; i < length; i++ {
					element, _ := vmInstance.GetProperty(arrayLike, fmt.Sprintf("%d", i))
					// Apply mapping function if provided
					if mapFn.Type() != vm.TypeUndefined {
						vmInstance.EnterHelperCall()
						mapped, mapErr := vmInstance.CallArgs2(mapFn, thisArg, element, vm.NumberValue(float64(i)))
						vmInstance.ExitHelperCall()
						if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
							return vm.NewArray(), nil
						}
						if mapErr != nil {
							return vm.NewArray(), mapErr
						}
						result.AsArray().Append(mapped)
					} else {
						result.AsArray().Append(element)
					}
				}
				return result, nil
			}
		}

		// Fallback: return empty array
		return vm.NewArray(), nil
	}))

	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("of", vm.NewNativeFunction(0, true, "of", func(args []vm.Value) (vm.Value, error) {
		// Array.of ( ...items )
		// 1. Let len be the actual number of arguments passed to this function.
		len := len(args)

		// 3. Let C be the this value.
		C := vmInstance.GetThis()

		var A vm.Value

		// 4. If IsConstructor(C) is true, then
		//    a. Let A be ? Construct(C, « len »).
		if C.IsCallable() && vmInstance.IsConstructor(C) {
			// Call C as constructor with len as single argument
			result, err := vmInstance.Construct(C, []vm.Value{vm.NumberValue(float64(len))})
			if err != nil {
				return vm.Undefined, err
			}
			A = result
		} else {
			// 5. Else,
			//    a. Let A be ? ArrayCreate(len).
			A = vm.NewArrayWithLength(len)
		}

		// 6. Let k be 0.
		// 7. Repeat, while k < len
		for k := 0; k < len; k++ {
			// a. Let kValue be items[k].
			kValue := args[k]
			// b. Let Pk be ! ToString(k).
			pk := fmt.Sprintf("%d", k)
			// c. Perform ? CreateDataPropertyOrThrow(A, Pk, kValue).
			if A.IsArray() {
				arr := A.AsArray()
				arr.Set(k, kValue)
			} else if A.Type() == vm.TypeObject {
				po := A.AsPlainObject()
				// Check if we can create/update this property
				// CreateDataPropertyOrThrow fails if:
				// 1. Object is non-extensible and property doesn't exist
				// 2. Property exists but is non-configurable
				// GetOwnDescriptor returns: (Value, writable, enumerable, configurable, exists)
				_, _, _, configurable, existingProp := po.GetOwnDescriptor(pk)
				if !existingProp && !po.IsExtensible() {
					return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("Cannot add property %s, object is not extensible", pk))
				}
				if existingProp && !configurable {
					return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("Cannot redefine property: %s", pk))
				}
				w, e, c := true, true, true // writable, enumerable, configurable
				po.DefineOwnProperty(pk, kValue, &w, &e, &c)
			}
		}

		// 8. Perform ? Set(A, "length", len, true).
		if A.IsArray() {
			// Already set by Set
		} else if A.Type() == vm.TypeObject {
			po := A.AsPlainObject()
			lengthVal := vm.NumberValue(float64(len))
			// Check if there's a setter for "length" and call it
			if _, setter, _, _, ok := po.GetOwnAccessor("length"); ok && setter.Type() != vm.TypeUndefined {
				_, err := vmInstance.Call(setter, A, []vm.Value{lengthVal})
				if err != nil {
					return vm.Undefined, err
				}
			} else {
				po.SetOwn("length", lengthVal)
			}
		}

		// 9. Return A.
		return A, nil
	}))

	// Array.fromAsync ( asyncItems [ , mapfn [ , thisArg ] ] )
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("fromAsync", vm.NewNativeFunction(1, false, "fromAsync", func(args []vm.Value) (vm.Value, error) {
		// Get asyncItems
		var asyncItems vm.Value = vm.Undefined
		if len(args) > 0 {
			asyncItems = args[0]
		}

		// Check if asyncItems is undefined or null
		if asyncItems.Type() == vm.TypeUndefined || asyncItems.Type() == vm.TypeNull {
			// Return a rejected promise with TypeError
			// Reject with the real TypeError object, not its message string
			// (#147): vm.NewString(reason.Error()) handed .catch() a bare
			// string, so e instanceof TypeError and e.message were both wrong.
			reason := vmInstance.NewTypeError("Cannot convert undefined or null to object")
			return vmInstance.NewRejectedPromise(vmInstance.ExceptionValueFromError(reason)), nil
		}

		// Get mapfn (optional)
		var mapfn vm.Value = vm.Undefined
		if len(args) > 1 {
			mapfn = args[1]
		}

		// Get thisArg (optional)
		var thisArg vm.Value = vm.Undefined
		if len(args) > 2 {
			thisArg = args[2]
		}

		// If mapfn is not undefined and not callable, throw TypeError
		if mapfn.Type() != vm.TypeUndefined && !mapfn.IsCallable() {
			reason := vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", mapfn.Type().String()))
			return vmInstance.NewRejectedPromise(vmInstance.ExceptionValueFromError(reason)), nil
		}

		// Get C (the this value - for subclass support)
		C := vmInstance.GetThis()

		// Create the promise that will be returned
		// We need to set up async iteration or array-like processing
		// This is done inside a promise executor

		// Create pending promise and resolve/reject functions
		promise := vmInstance.NewPendingPromise()
		promiseObj := promise.AsPromise()

		// Helper function to resolve the promise with the result array
		resolveWithArray := func(arr vm.Value) {
			vmInstance.ResolvePromise(promiseObj, arr)
		}

		// Helper function to reject the promise
		rejectWithError := func(err error) {
			vmInstance.RejectPromise(promiseObj, vm.NewString(err.Error()))
		}

		// Start async processing
		rt := vmInstance.GetAsyncRuntime()
		rt.ScheduleMicrotask(func() {
			// Try to get async iterator first
			var iteratorMethod vm.Value = vm.Undefined
			var usingAsyncIterator bool = false

			// Check for Symbol.asyncIterator using the VM's GetSymbolProperty
			if method, ok := vmInstance.GetSymbolProperty(asyncItems, SymbolAsyncIterator); ok && method.IsCallable() {
				iteratorMethod = method
				usingAsyncIterator = true
			}

			// If no async iterator, check for sync iterator
			if iteratorMethod.Type() == vm.TypeUndefined || !iteratorMethod.IsCallable() {
				usingAsyncIterator = false
				if method, ok := vmInstance.GetSymbolProperty(asyncItems, SymbolIterator); ok && method.IsCallable() {
					iteratorMethod = method
				}
			}

			// Create result array using constructor if appropriate
			var A vm.Value
			if C.IsCallable() && vmInstance.IsConstructor(C) {
				// Use the constructor
				result, err := vmInstance.Construct(C, []vm.Value{vm.NumberValue(0)})
				if err != nil {
					rejectWithError(err)
					return
				}
				A = result
			} else {
				A = vm.NewArray()
			}

			// Process using iterator or array-like
			if iteratorMethod.IsCallable() {
				// Use iterator
				iterator, err := vmInstance.Call(iteratorMethod, asyncItems, []vm.Value{})
				if err != nil {
					rejectWithError(err)
					return
				}

				// Get next method - handle different iterator types
				var nextMethod vm.Value = vm.Undefined

				// For plain objects, use GetProperty
				if iterator.Type() == vm.TypeObject {
					nextMethod, err = vmInstance.GetProperty(iterator, "next")
					if err != nil {
						rejectWithError(err)
						return
					}
				} else if iterator.Type() == vm.TypeGenerator {
					// For generators, get from GeneratorPrototype
					if vmInstance.GeneratorPrototype.Type() != vm.TypeUndefined {
						proto := vmInstance.GeneratorPrototype.AsPlainObject()
						if proto != nil {
							if m, ok := proto.Get("next"); ok {
								nextMethod = m
							}
						}
					}
				} else if iterator.Type() == vm.TypeAsyncGenerator {
					// For async generators, get from AsyncGeneratorPrototype
					if vmInstance.AsyncGeneratorPrototype.Type() != vm.TypeUndefined {
						proto := vmInstance.AsyncGeneratorPrototype.AsPlainObject()
						if proto != nil {
							if m, ok := proto.Get("next"); ok {
								nextMethod = m
							}
						}
					}
				}

				if !nextMethod.IsCallable() {
					rejectWithError(errors.New("iterator.next is not a function"))
					return
				}

				// Iterate asynchronously
				k := 0
				var iterateNext func()
				iterateNext = func() {
					// Call next()
					nextResult, err := vmInstance.Call(nextMethod, iterator, []vm.Value{})
					if err != nil {
						rejectWithError(err)
						return
					}

					// Handle async iterator (nextResult might be a promise)
					var handleNextResult func(result vm.Value)
					handleNextResult = func(result vm.Value) {
						// Get done and value
						var done bool = false
						var value vm.Value = vm.Undefined

						if result.Type() == vm.TypeObject {
							obj := result.AsPlainObject()
							if obj != nil {
								if doneVal, ok := obj.GetOwn("done"); ok {
									done = doneVal.IsTruthy()
								}
								if v, ok := obj.GetOwn("value"); ok {
									value = v
								}
							}
						}

						if done {
							// Set length and resolve
							if A.IsArray() {
								// Length is automatically updated for arrays
							} else if A.Type() == vm.TypeObject {
								A.AsPlainObject().SetOwn("length", vm.NumberValue(float64(k)))
							}
							resolveWithArray(A)
							return
						}

						// For async iterator, the value might be a promise
						var handleValue func(val vm.Value)
						handleValue = func(val vm.Value) {
							// Apply mapfn if present
							var mappedValue vm.Value = val
							if mapfn.IsCallable() {
								result, err := vmInstance.CallArgs2(mapfn, thisArg, val, vm.NumberValue(float64(k)))
								if err != nil {
									rejectWithError(err)
									return
								}
								mappedValue = result
							}

							// If mappedValue is a promise, wait for it
							if mappedValue.Type() == vm.TypePromise {
								mp := mappedValue.AsPromise()
								if mp != nil && mp.GetState() == vm.PromisePending {
									vmInstance.AddPromiseReaction(mappedValue, true, func(v vm.Value) {
										// Add to result array
										if A.IsArray() {
											A.AsArray().Set(k, v)
										} else if A.Type() == vm.TypeObject {
											A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), v)
										}
										k++
										iterateNext()
									})
									vmInstance.AddPromiseReaction(mappedValue, false, func(r vm.Value) {
										rejectWithError(errors.New(r.ToString()))
									})
									return
								} else if mp != nil && mp.GetState() == vm.PromiseFulfilled {
									mappedValue = mp.GetResult()
								} else if mp != nil && mp.GetState() == vm.PromiseRejected {
									rejectWithError(errors.New(mp.GetResult().ToString()))
									return
								}
							}

							// Add to result array
							if A.IsArray() {
								A.AsArray().Set(k, mappedValue)
							} else if A.Type() == vm.TypeObject {
								A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), mappedValue)
							}
							k++
							rt.ScheduleMicrotask(iterateNext)
						}

						// If value is a promise (for sync iterator with promise elements), wait for it
						if value.Type() == vm.TypePromise {
							vp := value.AsPromise()
							if vp != nil && vp.GetState() == vm.PromisePending {
								vmInstance.AddPromiseReaction(value, true, handleValue)
								vmInstance.AddPromiseReaction(value, false, func(r vm.Value) {
									rejectWithError(errors.New(r.ToString()))
								})
								return
							} else if vp != nil && vp.GetState() == vm.PromiseFulfilled {
								value = vp.GetResult()
							} else if vp != nil && vp.GetState() == vm.PromiseRejected {
								rejectWithError(errors.New(vp.GetResult().ToString()))
								return
							}
						}
						handleValue(value)
					}

					// If nextResult is a promise (async iterator), wait for it
					if usingAsyncIterator && nextResult.Type() == vm.TypePromise {
						np := nextResult.AsPromise()
						if np != nil && np.GetState() == vm.PromisePending {
							vmInstance.AddPromiseReaction(nextResult, true, handleNextResult)
							vmInstance.AddPromiseReaction(nextResult, false, func(r vm.Value) {
								rejectWithError(errors.New(r.ToString()))
							})
							return
						} else if np != nil && np.GetState() == vm.PromiseFulfilled {
							nextResult = np.GetResult()
						} else if np != nil && np.GetState() == vm.PromiseRejected {
							rejectWithError(errors.New(np.GetResult().ToString()))
							return
						}
					}
					handleNextResult(nextResult)
				}

				iterateNext()
			} else {
				// Array-like: use length property. Covers real Arrays,
				// PlainObjects, and everything else with a "length" (Arguments,
				// TypedArray, Proxy, ...) via the same generic accessor the
				// synchronous Array.prototype methods use.
				if !asyncItems.IsObject() && !asyncItems.IsCallable() {
					// Primitives with no [Symbol.asyncIterator]/[Symbol.iterator]
					// and nothing shaped like an array-like: treat as empty.
					resolveWithArray(A)
					return
				}
				length, lenErr := arrayLikeLength(vmInstance, asyncItems)
				if lenErr != nil {
					rejectWithError(lenErr)
					return
				}

				if length == 0 {
					resolveWithArray(A)
					return
				}

				// Process array-like elements
				k := 0
				var processNext func()
				processNext = func() {
					if k >= length {
						// Set length and resolve
						if A.IsArray() {
							// Length is auto-updated
						} else if A.Type() == vm.TypeObject {
							A.AsPlainObject().SetOwn("length", vm.NumberValue(float64(k)))
						}
						resolveWithArray(A)
						return
					}

					// Get element at k
					kValue, _, elemErr := arrayLikeGet(vmInstance, asyncItems, k)
					if elemErr != nil {
						rejectWithError(elemErr)
						return
					}

					// Handle the value (might be a promise)
					var handleKValue func(val vm.Value)
					handleKValue = func(val vm.Value) {
						// Apply mapfn if present
						var mappedValue vm.Value = val
						if mapfn.IsCallable() {
							result, err := vmInstance.CallArgs2(mapfn, thisArg, val, vm.NumberValue(float64(k)))
							if err != nil {
								rejectWithError(err)
								return
							}
							mappedValue = result
						}

						// If mappedValue is a promise, wait for it
						if mappedValue.Type() == vm.TypePromise {
							mp := mappedValue.AsPromise()
							if mp != nil && mp.GetState() == vm.PromisePending {
								vmInstance.AddPromiseReaction(mappedValue, true, func(v vm.Value) {
									// Add to result array
									if A.IsArray() {
										A.AsArray().Set(k, v)
									} else if A.Type() == vm.TypeObject {
										A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), v)
									}
									k++
									processNext()
								})
								vmInstance.AddPromiseReaction(mappedValue, false, func(r vm.Value) {
									rejectWithError(errors.New(r.ToString()))
								})
								return
							} else if mp != nil && mp.GetState() == vm.PromiseFulfilled {
								mappedValue = mp.GetResult()
							} else if mp != nil && mp.GetState() == vm.PromiseRejected {
								rejectWithError(errors.New(mp.GetResult().ToString()))
								return
							}
						}

						// Add to result array
						if A.IsArray() {
							A.AsArray().Set(k, mappedValue)
						} else if A.Type() == vm.TypeObject {
							A.AsPlainObject().SetOwn(fmt.Sprintf("%d", k), mappedValue)
						}
						k++
						rt.ScheduleMicrotask(processNext)
					}

					// If kValue is a promise, wait for it
					if kValue.Type() == vm.TypePromise {
						kp := kValue.AsPromise()
						if kp != nil && kp.GetState() == vm.PromisePending {
							vmInstance.AddPromiseReaction(kValue, true, handleKValue)
							vmInstance.AddPromiseReaction(kValue, false, func(r vm.Value) {
								rejectWithError(errors.New(r.ToString()))
							})
							return
						} else if kp != nil && kp.GetState() == vm.PromiseFulfilled {
							kValue = kp.GetResult()
						} else if kp != nil && kp.GetState() == vm.PromiseRejected {
							rejectWithError(errors.New(kp.GetResult().ToString()))
							return
						}
					}
					handleKValue(kValue)
				}

				processNext()
			}
		})

		return promise, nil
	}))

	// Array.prototype.values() - returns iterator yielding values
	// This is the same function as [Symbol.iterator] per ECMAScript spec
	valuesFn := vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// First check for array type (AsArray() panics on wrong type, so check type first)
		if thisVal.Type() == vm.TypeArray {
			return createArrayIterator(vmInstance, thisVal.AsArray()), nil
		}

		// Handle Arguments objects (array-like)
		if thisVal.Type() == vm.TypeArguments {
			argsObj := thisVal.AsArguments()
			return createArgumentsIterator(vmInstance, argsObj), nil
		}

		// Handle generic array-like objects with length property (Map/Set,
		// TypedArray, Proxy, boxed primitives, ...) - arrayLikeLength returns
		// 0 for anything with no "length" property rather than erroring, so
		// this covers exactly the same "has a length property" test the old
		// PlainObject-only check did, without panicking on anything that
		// isn't a plain Object.
		if thisVal.IsObject() {
			if _, err := arrayLikeLength(vmInstance, thisVal); err == nil {
				return createArrayLikeIterator(vmInstance, thisVal), nil
			}
		}

		return vm.Undefined, nil
	})
	arrayProto.SetOwnNonEnumerable("values", valuesFn)

	// [Symbol.iterator] is the same function as values() per ECMAScript spec
	// Native symbol key - make it writable and configurable like standard JavaScript
	w, e, c := true, false, true // writable, not enumerable, configurable
	arrayProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), valuesFn, &w, &e, &c)

	// Record the canonical values/[Symbol.iterator] so the compiler's
	// array-destructuring fast path (OpArrayDestructFastCheck) can identity-check
	// against it and skip the iterator protocol for pristine arrays.
	vmInstance.ArrayValuesIterator = valuesFn

	// Array.prototype.keys() - returns iterator yielding indices
	keysFn := vm.NewNativeFunction(0, false, "keys", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		return createArrayKeysIterator(vmInstance, thisVal), nil
	})
	arrayProto.SetOwnNonEnumerable("keys", keysFn)

	// Array.prototype.entries() - returns iterator yielding [index, value] pairs
	entriesFn := vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}
		return createArrayEntriesIterator(vmInstance, thisVal), nil
	})
	arrayProto.SetOwnNonEnumerable("entries", entriesFn)

	// Add Symbol.asyncIterator implementation for arrays (for await...of support)
	// This wraps the sync iterator in an async iterator (returns promises)
	asyncIterFn := vm.NewNativeFunction(0, false, "[Symbol.asyncIterator]", func(args []vm.Value) (vm.Value, error) {
		thisVal := vmInstance.GetThis()
		if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
			return vm.Undefined, vmInstance.NewTypeError("Cannot convert undefined or null to object")
		}

		// Create an async array iterator object (wraps sync iteration)
		return createAsyncArrayIterator(vmInstance, thisVal), nil
	})
	// Make it writable and configurable like standard JavaScript
	w2, e2, c2 := true, false, true
	arrayProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolAsyncIterator), asyncIterFn, &w2, &e2, &c2)

	arrayCtor := ctorWithProps

	// Set constructor property on Array.prototype to point to Array constructor
	arrayProto.SetOwnNonEnumerable("constructor", arrayCtor)
	// Make it non-enumerable like in standard JavaScript
	if v, ok := arrayProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		arrayProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Add Symbol.unscopables per ECMAScript spec
	// This object has properties that should be excluded from `with` statement bindings
	unscopablesObj := vm.NewObject(vm.Null).AsPlainObject() // null prototype per spec
	unscopablesObj.SetOwn("at", vm.True)
	unscopablesObj.SetOwn("copyWithin", vm.True)
	unscopablesObj.SetOwn("entries", vm.True)
	unscopablesObj.SetOwn("fill", vm.True)
	unscopablesObj.SetOwn("find", vm.True)
	unscopablesObj.SetOwn("findIndex", vm.True)
	unscopablesObj.SetOwn("findLast", vm.True)
	unscopablesObj.SetOwn("findLastIndex", vm.True)
	unscopablesObj.SetOwn("flat", vm.True)
	unscopablesObj.SetOwn("flatMap", vm.True)
	unscopablesObj.SetOwn("includes", vm.True)
	unscopablesObj.SetOwn("keys", vm.True)
	unscopablesObj.SetOwn("toReversed", vm.True)
	unscopablesObj.SetOwn("toSorted", vm.True)
	unscopablesObj.SetOwn("toSpliced", vm.True)
	unscopablesObj.SetOwn("values", vm.True)
	// Make Symbol.unscopables non-writable, non-enumerable, configurable
	wU, eU, cU := false, false, true
	arrayProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolUnscopables), vm.NewValueFromPlainObject(unscopablesObj), &wU, &eU, &cU)

	// Set Array prototype in VM
	vmInstance.ArrayPrototype = vm.NewValueFromPlainObject(arrayProto)

	// Register Array constructor as global
	return ctx.DefineGlobal("Array", arrayCtor)
}

// makeBuiltinIterNext builds the standard next() closure over shared iterator
// state and tags its NativeFunctionObject so the VM's for-of fast path
// (OpIterFastCheck/OpFastIterNext) can step the same state without a call or
// {value, done} allocation. Manual next() calls and the fast path advance one
// shared index.
func makeBuiltinIterNext(vmInstance *vm.VM, state *vm.BuiltinIterState) vm.Value {
	nextFn := vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		v, done := state.Step()
		result.SetOwnNonEnumerable("value", v)
		result.SetOwnNonEnumerable("done", vm.BooleanValue(done))
		return vm.NewValueFromPlainObject(result), nil
	})
	nextFn.AsNativeFunction().IterState = state
	return nextFn
}

// createArrayIterator creates an iterator object for array iteration
func createArrayIterator(vmInstance *vm.VM, array *vm.ArrayObject) vm.Value {
	// Create iterator object inheriting from ArrayIteratorPrototype
	iterator := vm.NewObject(vmInstance.ArrayIteratorPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	state := &vm.BuiltinIterState{Kind: vm.IterKindArrayValues, Arr: array}
	iterator.SetOwnNonEnumerable("next", makeBuiltinIterNext(vmInstance, state))

	// Add [Symbol.iterator] that returns the iterator itself (required for for-of)
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArgumentsIterator creates an iterator object for Arguments objects
func createArgumentsIterator(vmInstance *vm.VM, args *vm.ArgumentsObject) vm.Value {
	// Create iterator object inheriting from Iterator.prototype
	iterator := vm.NewObject(vmInstance.ArrayIteratorPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	state := &vm.BuiltinIterState{Kind: vm.IterKindArguments, Args: args}
	iterator.SetOwnNonEnumerable("next", makeBuiltinIterNext(vmInstance, state))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArrayLikeIterator creates an iterator for generic array-like objects (with length and indices)
func createArrayLikeIterator(vmInstance *vm.VM, arrayLike vm.Value) vm.Value {
	// Create iterator object inheriting from Iterator.prototype
	iterator := vm.NewObject(vmInstance.ArrayIteratorPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	state := &vm.BuiltinIterState{Kind: vm.IterKindLikeValues, Like: asPlainObjectOrNil(arrayLike)}
	iterator.SetOwnNonEnumerable("next", makeBuiltinIterNext(vmInstance, state))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArrayKeysIterator creates an iterator that yields array indices
func createArrayKeysIterator(vmInstance *vm.VM, arrayLike vm.Value) vm.Value {
	iterator := vm.NewObject(vmInstance.ArrayIteratorPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	state := &vm.BuiltinIterState{Kind: vm.IterKindArrayKeys}
	if arrayLike.Type() == vm.TypeArray {
		state.Arr = arrayLike.AsArray()
	} else {
		state.Like = asPlainObjectOrNil(arrayLike) // nil source (or non-PlainObject array-like) iterates as length 0
	}
	iterator.SetOwnNonEnumerable("next", makeBuiltinIterNext(vmInstance, state))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createArrayEntriesIterator creates an iterator that yields [index, value] pairs
func createArrayEntriesIterator(vmInstance *vm.VM, arrayLike vm.Value) vm.Value {
	iterator := vm.NewObject(vmInstance.ArrayIteratorPrototype).AsPlainObject()
	iteratorVal := vm.NewValueFromPlainObject(iterator)

	state := &vm.BuiltinIterState{Kind: vm.IterKindArrayEntries}
	if arrayLike.Type() == vm.TypeArray {
		state.Arr = arrayLike.AsArray()
	} else {
		state.Like = asPlainObjectOrNil(arrayLike) // nil source (or non-PlainObject array-like) iterates as length 0
	}
	iterator.SetOwnNonEnumerable("next", makeBuiltinIterNext(vmInstance, state))

	// Add [Symbol.iterator] that returns the iterator itself
	iterSelfFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(fnArgs []vm.Value) (vm.Value, error) {
		return iteratorVal, nil
	})
	w, e, c := true, false, true
	iterator.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), iterSelfFn, &w, &e, &c)

	return iteratorVal
}

// createAsyncArrayIterator creates an async iterator object for array iteration
// This wraps array iteration to return promises (for await...of support)
// createAsyncArrayIterator wraps thisVal's synchronous iteration (via the
// same arrayLikeLength/arrayLikeGet generic accessors every other
// Array.prototype method uses) in an async iterator whose next() resolves a
// Promise<{value, done}> - not just for real Arrays, but for anything
// [Symbol.asyncIterator] can legally be called on via .call()/.apply()
// (Arguments, TypedArray, Proxy, array-likes, ...).
func createAsyncArrayIterator(vmInstance *vm.VM, thisVal vm.Value) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator - returns Promise<{value, done}>
	iterator.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		length, err := arrayLikeLength(vmInstance, thisVal)
		if err != nil {
			return vm.Undefined, err
		}
		if currentIndex >= length {
			// Iterator is exhausted
			result.SetOwnNonEnumerable("value", vm.Undefined)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(true))
		} else {
			// Return current element and advance
			val, _, err := arrayLikeGet(vmInstance, thisVal, currentIndex)
			if err != nil {
				return vm.Undefined, err
			}
			result.SetOwnNonEnumerable("value", val)
			result.SetOwnNonEnumerable("done", vm.BooleanValue(false))
			currentIndex++
		}

		// Wrap result in a resolved promise (async iterator protocol)
		resultVal := vm.NewValueFromPlainObject(result)
		return vmInstance.NewResolvedPromise(resultVal), nil
	}))

	return vm.NewValueFromPlainObject(iterator)
}

// Helper methods for creating generic array method types

// createGenericMethod creates a generic method with a single type parameter T
func (a *ArrayInitializer) createGenericMethod(name string, tParam *types.TypeParameter, methodType types.Type) types.Type {
	return &types.GenericType{
		Name:           name,
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           methodType,
	}
}

// createGenericMapMethod creates the special map method that has two type parameters T and U
func (a *ArrayInitializer) createGenericMapMethod(tParam *types.TypeParameter) types.Type {
	// For map, we need both T (input element type) and U (output element type)
	uParam := &types.TypeParameter{Name: "U", Constraint: nil, Index: 1}
	uType := &types.TypeParameterType{Parameter: uParam}
	tType := &types.TypeParameterType{Parameter: tParam}
	tArrayType := &types.ArrayType{ElementType: tType}
	uArrayType := &types.ArrayType{ElementType: uType}

	// map<U>((value: T, index?: number, array?: T[]) => U): U[]
	callbackType := types.NewOptionalFunction(
		[]types.Type{tType, types.Number, tArrayType},
		uType,
		[]bool{false, true, true})

	methodType := types.NewSimpleFunction([]types.Type{callbackType}, uArrayType)

	return &types.GenericType{
		Name:           "map",
		TypeParameters: []*types.TypeParameter{tParam, uParam},
		Body:           methodType,
	}
}

package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

type SetInitializer struct{}

func (s *SetInitializer) Name() string {
	return "Set"
}

func (s *SetInitializer) Priority() int {
	return 410 // After Map (400)
}

func (s *SetInitializer) InitTypes(ctx *TypeContext) error {
	// Create generic type parameter T for set methods
	tParam := &types.TypeParameter{Name: "T", Constraint: nil, Index: 0}
	tType := &types.TypeParameterType{Parameter: tParam}

	// Create the generic type first (with placeholder body)
	setType := &types.GenericType{
		Name:           "Set",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           nil, // Will be set below
	}

	// Create Set instance type with methods (using type parameters directly)
	// forEach callback: (value: T, value2: T, set: Set<T>) => void
	forEachCallbackType := types.NewSimpleFunction([]types.Type{tType, tType, setType}, types.Void)

	setInstanceType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)). // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{forEachCallbackType}, types.Void)).
		WithProperty("size", types.Number)

	// Now set the body of the generic type
	setType.Body = setInstanceType

	// Create Set.prototype type for runtime (same structure)
	setProtoType := types.NewObjectType().
		WithProperty("add", types.NewSimpleFunction([]types.Type{tType}, setType)). // Return this for chaining
		WithProperty("has", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("delete", types.NewSimpleFunction([]types.Type{tType}, types.Boolean)).
		WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
		WithProperty("forEach", types.NewSimpleFunction([]types.Type{forEachCallbackType}, types.Void)).
		WithProperty("size", types.Number)

	// Register set primitive prototype
	ctx.SetPrimitivePrototype("set", setProtoType)

	// Create Set constructor type - use a generic constructor
	setCtorType := &types.GenericType{
		Name:           "Set",
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           types.NewSimpleFunction([]types.Type{}, setType),
	}

	// Define Set constructor in global environment
	err := ctx.DefineGlobal("Set", setCtorType)
	if err != nil {
		return err
	}

	// Also define the type alias for type annotations like Set<string>
	return ctx.DefineTypeAlias("Set", setType)
}

// createSetMethod creates a generic method with T type parameter
func (s *SetInitializer) createSetMethod(name string, tParam *types.TypeParameter, methodType types.Type) types.Type {
	return &types.GenericType{
		Name:           name,
		TypeParameters: []*types.TypeParameter{tParam},
		Body:           methodType,
	}
}

func (s *SetInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create Set.prototype inheriting from Object.prototype
	setProto := vm.NewObject(objectProto).AsPlainObject()

	// Add Set prototype methods
	setProto.SetOwnNonEnumerable("add", vm.NewNativeFunction(1, false, "add", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.add called on incompatible receiver")
		}

		if len(args) < 1 {
			return thisSet, nil // Return the set for chaining
		}

		setObj := thisSet.AsSet()
		setObj.Add(args[0])
		return thisSet, nil // Return the set for chaining
	}))
	if v, ok := setProto.GetOwn("add"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("add", v, &w, &e, &c)
	}

	setProto.SetOwnNonEnumerable("has", vm.NewNativeFunction(1, false, "has", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.has called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		setObj := thisSet.AsSet()
		return vm.BooleanValue(setObj.Has(args[0])), nil
	}))
	if v, ok := setProto.GetOwn("has"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("has", v, &w, &e, &c)
	}

	setProto.SetOwnNonEnumerable("delete", vm.NewNativeFunction(1, false, "delete", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.delete called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}

		setObj := thisSet.AsSet()
		return vm.BooleanValue(setObj.Delete(args[0])), nil
	}))
	if v, ok := setProto.GetOwn("delete"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("delete", v, &w, &e, &c)
	}

	setProto.SetOwnNonEnumerable("clear", vm.NewNativeFunction(0, false, "clear", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()

		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.clear called on incompatible receiver")
		}

		setObj := thisSet.AsSet()
		setObj.Clear()
		return vm.Undefined, nil
	}))
	if v, ok := setProto.GetOwn("clear"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("clear", v, &w, &e, &c)
	}

	// Helper function to get a SetRecord from a set-like object
	// Per ECMAScript spec: GetSetRecord(obj) returns { set, size, has, keys }
	getSetRecord := func(other vm.Value, methodName string) (int, vm.Value, vm.Value, error) {
		if !other.IsObject() {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + " requires an object argument")
		}

		// Get size property and coerce to number
		// Per spec: if rawSize is undefined, throw TypeError
		sizeVal, err := vmInstance.GetProperty(other, "size")
		if err != nil {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + ": argument must have a size property")
		}
		if sizeVal.Type() == vm.TypeUndefined {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + ": argument must have a size property")
		}
		// Per spec: BigInt throws TypeError
		if sizeVal.Type() == vm.TypeBigInt {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + ": Cannot convert a BigInt to a number")
		}
		// Use ToNumber which properly calls valueOf for objects
		sizeFloat := vmInstance.ToNumber(sizeVal)
		// Per spec: NaN throws TypeError, negative throws RangeError
		if sizeFloat != sizeFloat { // NaN check
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + ": size is not a valid number")
		}
		if sizeFloat < 0 {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewRangeError(methodName + ": size cannot be negative")
		}
		size := int(sizeFloat)

		// Get has method
		hasMethod, err := vmInstance.GetProperty(other, "has")
		if err != nil || !hasMethod.IsCallable() {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + ": argument must have a callable has method")
		}

		// Get keys method
		keysMethod, err := vmInstance.GetProperty(other, "keys")
		if err != nil || !keysMethod.IsCallable() {
			return 0, vm.Undefined, vm.Undefined, vmInstance.NewTypeError(methodName + ": argument must have a callable keys method")
		}

		return size, hasMethod, keysMethod, nil
	}

	// Helper to iterate over keys from a set-like object
	iterateSetLike := func(other vm.Value, keysMethod vm.Value, callback func(vm.Value) (bool, error)) error {
		// Call keys() to get iterator
		iter, err := vmInstance.Call(keysMethod, other, nil)
		if err != nil {
			return err
		}

		// Get next method from iterator
		nextMethod, err := vmInstance.GetProperty(iter, "next")
		if err != nil || !nextMethod.IsCallable() {
			return vmInstance.NewTypeError("keys iterator must have a next method")
		}

		// Iterate
		for {
			result, err := vmInstance.Call(nextMethod, iter, nil)
			if err != nil {
				return err
			}

			// Check done
			doneVal, _ := vmInstance.GetProperty(result, "done")
			if doneVal.IsTruthy() {
				break
			}

			// Get value
			value, _ := vmInstance.GetProperty(result, "value")

			// Call callback
			cont, err := callback(value)
			if err != nil {
				return err
			}
			if !cont {
				break
			}
		}
		return nil
	}

	// Set.prototype.union(other) - ES2024
	setProto.SetOwnNonEnumerable("union", vm.NewNativeFunction(1, false, "union", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.union called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.union requires an argument")
		}

		_, _, keysMethod, err := getSetRecord(args[0], "Set.prototype.union")
		if err != nil {
			return vm.Undefined, err
		}

		// Create new Set with all elements from this
		result := vm.NewSet()
		resultSet := result.AsSet()
		thisSetObj := thisSet.AsSet()
		for i := 0; i < thisSetObj.OrderLen(); i++ {
			if val, exists := thisSetObj.GetValueAt(i); exists {
				resultSet.Add(val)
			}
		}

		// Add all elements from other
		err = iterateSetLike(args[0], keysMethod, func(val vm.Value) (bool, error) {
			resultSet.Add(val)
			return true, nil
		})
		if err != nil {
			return vm.Undefined, err
		}

		return result, nil
	}))
	if v, ok := setProto.GetOwn("union"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("union", v, &w, &e, &c)
	}

	// Set.prototype.intersection(other) - ES2024
	setProto.SetOwnNonEnumerable("intersection", vm.NewNativeFunction(1, false, "intersection", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.intersection called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.intersection requires an argument")
		}

		otherSize, hasMethod, keysMethod, err := getSetRecord(args[0], "Set.prototype.intersection")
		if err != nil {
			return vm.Undefined, err
		}

		result := vm.NewSet()
		resultSet := result.AsSet()
		thisSetObj := thisSet.AsSet()

		// Optimization: iterate over the smaller set
		if thisSetObj.Size() <= otherSize {
			// Iterate over this, check has in other
			for i := 0; i < thisSetObj.OrderLen(); i++ {
				if val, exists := thisSetObj.GetValueAt(i); exists {
					hasResult, err := vmInstance.Call(hasMethod, args[0], []vm.Value{val})
					if err != nil {
						return vm.Undefined, err
					}
					if hasResult.IsTruthy() {
						resultSet.Add(val)
					}
				}
			}
		} else {
			// Iterate over other, check has in this
			err = iterateSetLike(args[0], keysMethod, func(val vm.Value) (bool, error) {
				if thisSetObj.Has(val) {
					resultSet.Add(val)
				}
				return true, nil
			})
			if err != nil {
				return vm.Undefined, err
			}
		}

		return result, nil
	}))
	if v, ok := setProto.GetOwn("intersection"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("intersection", v, &w, &e, &c)
	}

	// Set.prototype.difference(other) - ES2024
	setProto.SetOwnNonEnumerable("difference", vm.NewNativeFunction(1, false, "difference", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.difference called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.difference requires an argument")
		}

		otherSize, hasMethod, keysMethod, err := getSetRecord(args[0], "Set.prototype.difference")
		if err != nil {
			return vm.Undefined, err
		}

		result := vm.NewSet()
		resultSet := result.AsSet()
		thisSetObj := thisSet.AsSet()

		// Copy all from this first
		for i := 0; i < thisSetObj.OrderLen(); i++ {
			if val, exists := thisSetObj.GetValueAt(i); exists {
				resultSet.Add(val)
			}
		}

		// Remove elements that are in other
		if thisSetObj.Size() <= otherSize {
			// Iterate this, check has in other, remove if found
			for i := 0; i < thisSetObj.OrderLen(); i++ {
				if val, exists := thisSetObj.GetValueAt(i); exists {
					hasResult, err := vmInstance.Call(hasMethod, args[0], []vm.Value{val})
					if err != nil {
						return vm.Undefined, err
					}
					if hasResult.IsTruthy() {
						resultSet.Delete(val)
					}
				}
			}
		} else {
			// Iterate other, remove from result
			err = iterateSetLike(args[0], keysMethod, func(val vm.Value) (bool, error) {
				resultSet.Delete(val)
				return true, nil
			})
			if err != nil {
				return vm.Undefined, err
			}
		}

		return result, nil
	}))
	if v, ok := setProto.GetOwn("difference"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("difference", v, &w, &e, &c)
	}

	// Set.prototype.symmetricDifference(other) - ES2024
	setProto.SetOwnNonEnumerable("symmetricDifference", vm.NewNativeFunction(1, false, "symmetricDifference", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.symmetricDifference called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.symmetricDifference requires an argument")
		}

		_, _, keysMethod, err := getSetRecord(args[0], "Set.prototype.symmetricDifference")
		if err != nil {
			return vm.Undefined, err
		}

		result := vm.NewSet()
		resultSet := result.AsSet()
		thisSetObj := thisSet.AsSet()

		// Add all from this
		for i := 0; i < thisSetObj.OrderLen(); i++ {
			if val, exists := thisSetObj.GetValueAt(i); exists {
				resultSet.Add(val)
			}
		}

		// For each element in other: if in result, remove; else add
		err = iterateSetLike(args[0], keysMethod, func(val vm.Value) (bool, error) {
			if resultSet.Has(val) {
				resultSet.Delete(val)
			} else {
				resultSet.Add(val)
			}
			return true, nil
		})
		if err != nil {
			return vm.Undefined, err
		}

		return result, nil
	}))
	if v, ok := setProto.GetOwn("symmetricDifference"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("symmetricDifference", v, &w, &e, &c)
	}

	// Set.prototype.isSubsetOf(other) - ES2024
	setProto.SetOwnNonEnumerable("isSubsetOf", vm.NewNativeFunction(1, false, "isSubsetOf", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.isSubsetOf called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.isSubsetOf requires an argument")
		}

		otherSize, hasMethod, _, err := getSetRecord(args[0], "Set.prototype.isSubsetOf")
		if err != nil {
			return vm.Undefined, err
		}

		thisSetObj := thisSet.AsSet()

		// If this is larger than other, can't be a subset
		if thisSetObj.Size() > otherSize {
			return vm.False, nil
		}

		// Check that every element of this is in other
		for i := 0; i < thisSetObj.OrderLen(); i++ {
			if val, exists := thisSetObj.GetValueAt(i); exists {
				hasResult, err := vmInstance.Call(hasMethod, args[0], []vm.Value{val})
				if err != nil {
					return vm.Undefined, err
				}
				if !hasResult.IsTruthy() {
					return vm.False, nil
				}
			}
		}

		return vm.True, nil
	}))
	if v, ok := setProto.GetOwn("isSubsetOf"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("isSubsetOf", v, &w, &e, &c)
	}

	// Set.prototype.isSupersetOf(other) - ES2024
	setProto.SetOwnNonEnumerable("isSupersetOf", vm.NewNativeFunction(1, false, "isSupersetOf", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.isSupersetOf called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.isSupersetOf requires an argument")
		}

		otherSize, _, keysMethod, err := getSetRecord(args[0], "Set.prototype.isSupersetOf")
		if err != nil {
			return vm.Undefined, err
		}

		thisSetObj := thisSet.AsSet()

		// If other is larger than this, can't be a superset
		if otherSize > thisSetObj.Size() {
			return vm.False, nil
		}

		// Check that every element of other is in this
		isSuperset := true
		err = iterateSetLike(args[0], keysMethod, func(val vm.Value) (bool, error) {
			if !thisSetObj.Has(val) {
				isSuperset = false
				return false, nil // Stop iteration
			}
			return true, nil
		})
		if err != nil {
			return vm.Undefined, err
		}

		return vm.BooleanValue(isSuperset), nil
	}))
	if v, ok := setProto.GetOwn("isSupersetOf"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("isSupersetOf", v, &w, &e, &c)
	}

	// Set.prototype.isDisjointFrom(other) - ES2024
	setProto.SetOwnNonEnumerable("isDisjointFrom", vm.NewNativeFunction(1, false, "isDisjointFrom", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.isDisjointFrom called on incompatible receiver")
		}

		if len(args) < 1 {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.isDisjointFrom requires an argument")
		}

		otherSize, hasMethod, keysMethod, err := getSetRecord(args[0], "Set.prototype.isDisjointFrom")
		if err != nil {
			return vm.Undefined, err
		}

		thisSetObj := thisSet.AsSet()

		// Optimization: iterate over the smaller set
		if thisSetObj.Size() <= otherSize {
			// Iterate this, check has in other
			for i := 0; i < thisSetObj.OrderLen(); i++ {
				if val, exists := thisSetObj.GetValueAt(i); exists {
					hasResult, err := vmInstance.Call(hasMethod, args[0], []vm.Value{val})
					if err != nil {
						return vm.Undefined, err
					}
					if hasResult.IsTruthy() {
						return vm.False, nil
					}
				}
			}
		} else {
			// Iterate other, check has in this
			isDisjoint := true
			err = iterateSetLike(args[0], keysMethod, func(val vm.Value) (bool, error) {
				if thisSetObj.Has(val) {
					isDisjoint = false
					return false, nil // Stop iteration
				}
				return true, nil
			})
			if err != nil {
				return vm.Undefined, err
			}
			if !isDisjoint {
				return vm.False, nil
			}
		}

		return vm.True, nil
	}))
	if v, ok := setProto.GetOwn("isDisjointFrom"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("isDisjointFrom", v, &w, &e, &c)
	}

	// forEach(callback, thisArg)
	setProto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(1, false, "forEach", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.forEach called on incompatible receiver")
		}

		if len(args) < 1 || !args[0].IsCallable() {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.forEach requires a callable function")
		}

		callback := args[0]
		var thisArg vm.Value
		if len(args) >= 2 {
			thisArg = args[1]
		} else {
			thisArg = vm.Undefined
		}

		setObj := thisSet.AsSet()
		// Iterate manually so we can propagate callback errors
		for i := 0; i < setObj.OrderLen(); i++ {
			val, exists := setObj.GetValueAt(i)
			if exists {
				// forEach callback receives (value, value, set) - value is passed twice for consistency with Map
				_, err := vmInstance.Call(callback, thisArg, []vm.Value{val, val, thisSet})
				if err != nil {
					return vm.Undefined, err
				}
			}
		}

		return vm.Undefined, nil
	}))
	if v, ok := setProto.GetOwn("forEach"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("forEach", v, &w, &e, &c)
	}

	// Minimal iterator helpers: values(), keys(), entries(), and [Symbol.iterator]
	// These create iterator objects with internal slots that the prototype's next() uses.
	// Per ECMAScript spec: Set.prototype.keys === Set.prototype.values === Set.prototype[@@iterator]
	valuesFn := vm.NewNativeFunction(0, false, "values", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.values called on incompatible receiver")
		}

		// Create iterator object with internal slots, inheriting from SetIteratorPrototype
		it := vm.NewObject(vmInstance.SetIteratorPrototype).AsPlainObject()
		it.SetOwn("[[IteratedSet]]", thisSet)
		it.SetOwn("[[SetNextIndex]]", vm.NumberValue(0))
		it.SetOwn("[[SetIterationKind]]", vm.NewString("values"))
		it.SetOwn("[[Exhausted]]", vm.BooleanValue(false))

		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	})
	// Set values, keys, and Symbol.iterator to the SAME function object
	setProto.SetOwnNonEnumerable("values", valuesFn)
	if v, ok := setProto.GetOwn("values"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("values", v, &w, &e, &c)
	}
	// keys() is the same function as values() per ECMAScript spec
	setProto.SetOwnNonEnumerable("keys", valuesFn)
	if v, ok := setProto.GetOwn("keys"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("keys", v, &w, &e, &c)
	}
	// Set.prototype[Symbol.iterator] is the same function as values() per ECMAScript spec
	wb, eb, cb := true, false, true
	setProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), valuesFn, &wb, &eb, &cb)

	// entries() yields [value, value] - uses internal slots pattern
	setProto.SetOwnNonEnumerable("entries", vm.NewNativeFunction(0, false, "entries", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.entries called on incompatible receiver")
		}

		// Create iterator object with internal slots, inheriting from SetIteratorPrototype
		it := vm.NewObject(vmInstance.SetIteratorPrototype).AsPlainObject()
		it.SetOwn("[[IteratedSet]]", thisSet)
		it.SetOwn("[[SetNextIndex]]", vm.NumberValue(0))
		it.SetOwn("[[SetIterationKind]]", vm.NewString("entries"))
		it.SetOwn("[[Exhausted]]", vm.BooleanValue(false))

		it.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(a []vm.Value) (vm.Value, error) {
			return vm.NewValueFromPlainObject(it), nil
		}), nil, nil, nil)
		return vm.NewValueFromPlainObject(it), nil
	}))
	if v, ok := setProto.GetOwn("entries"); ok {
		w, e, c := true, false, true
		setProto.DefineOwnProperty("entries", v, &w, &e, &c)
	}

	// Add size accessor (getter) - must be defined as an accessor property per spec
	sizeGetter := vm.NewNativeFunction(0, false, "get size", func(args []vm.Value) (vm.Value, error) {
		thisSet := vmInstance.GetThis()
		if thisSet.Type() != vm.TypeSet {
			return vm.Undefined, vmInstance.NewTypeError("Set.prototype.size called on incompatible receiver")
		}
		setObj := thisSet.AsSet()
		return vm.IntegerValue(int32(setObj.Size())), nil
	})
	e, c := false, true
	setProto.DefineAccessorProperty("size", sizeGetter, true, vm.Undefined, false, &e, &c)

	// Create Set constructor function (before setting prototype, so we can reference it)
	setConstructor := vm.NewConstructorWithProps(0, false, "Set", func(args []vm.Value) (vm.Value, error) {
		// Create new Set instance
		newSet := vm.NewSet()
		setObj := newSet.AsSet()

		// If an iterable argument is provided, add all its elements
		if len(args) > 0 && !args[0].IsUndefined() && args[0].Type() != vm.TypeNull {
			iterable := args[0]

			// Handle different iterable types
			switch iterable.Type() {
			case vm.TypeArray:
				// Array: iterate and add all elements
				arr := iterable.AsArray()
				for i := 0; i < arr.Length(); i++ {
					setObj.Add(arr.Get(i))
				}
			case vm.TypeString:
				// String: iterate and add each character
				str := iterable.AsString()
				for _, char := range str {
					setObj.Add(vm.NewString(string(char)))
				}
			case vm.TypeSet:
				// Set: copy all values from the source set
				srcSet := iterable.AsSet()
				srcSet.ForEach(func(val vm.Value) {
					setObj.Add(val)
				})
			case vm.TypeMap:
				// Map: add all [key, value] pairs
				srcMap := iterable.AsMap()
				srcMap.ForEach(func(key vm.Value, val vm.Value) {
					pairVal := vm.NewArray()
					pairArr := pairVal.AsArray()
					pairArr.Append(key)
					pairArr.Append(val)
					setObj.Add(pairVal)
				})
				// For other types, we'd need full iterator protocol support
				// For now, silently ignore non-iterable arguments
			}
		}

		return newSet, nil
	})

	// Set constructor property on Set.prototype to point to Set constructor
	setProto.SetOwnNonEnumerable("constructor", setConstructor)
	if v, ok := setProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		setProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Add Symbol.toStringTag to Set.prototype (writable: false, enumerable: false, configurable: true)
	wFalse, eFalse, cTrue := false, false, true
	setProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolToStringTag), vm.NewString("Set"), &wFalse, &eFalse, &cTrue)

	// Set Set.prototype in VM (must be before adding prototype property to constructor)
	vmInstance.SetPrototype = vm.NewValueFromPlainObject(setProto)

	// Add prototype property
	setConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vmInstance.SetPrototype)
	if v, ok := setConstructor.AsNativeFunctionWithProps().Properties.GetOwn("prototype"); ok {
		w, e, c := false, false, false
		setConstructor.AsNativeFunctionWithProps().Properties.DefineOwnProperty("prototype", v, &w, &e, &c)
	}

	// Define Set constructor in global scope
	return ctx.DefineGlobal("Set", setConstructor)
}

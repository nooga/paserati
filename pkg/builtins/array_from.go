package builtins

import (
	"strconv"

	"github.com/nooga/paserati/pkg/vm"
)

// arrayFromConstructor is Array.from ( items [ , mapfn [ , thisArg ] ] ) for a
// this value C that is a constructor other than %Array%: the result is
// Construct(C) (iterable path) or Construct(C, « len ») (array-like path),
// filled through CreateDataPropertyOrThrow and finished with
// Set(A, "length", len, true) (#555).
func arrayFromConstructor(vmi *vm.VM, c, items, mapFn, thisArg vm.Value) (vm.Value, error) {
	mapping := mapFn.Type() != vm.TypeUndefined
	mapValue := func(v vm.Value, k int) (vm.Value, error) {
		if !mapping {
			return v, nil
		}
		return vmi.Call(mapFn, thisArg, []vm.Value{v, vm.NumberValue(float64(k))})
	}

	usingIterator, err := getSymbolMethod(vmi, items, SymbolIterator)
	if err != nil {
		return vm.Undefined, err
	}

	if usingIterator.Type() != vm.TypeUndefined {
		a, err := vmi.Construct(c, nil)
		if err != nil {
			return vm.Undefined, err
		}
		iter, err := vmi.Call(usingIterator, items, nil)
		if err != nil {
			return vm.Undefined, err
		}
		if !isObjectValue(iter) {
			return vm.Undefined, vmi.NewTypeError("Result of the Symbol.iterator method is not an object")
		}
		next, err := vmi.GetProperty(iter, "next")
		if err != nil {
			return vm.Undefined, err
		}
		for k := 0; ; k++ {
			if !next.IsCallable() {
				return vm.Undefined, vmi.NewTypeError("iterator.next is not a function")
			}
			res, err := vmi.Call(next, iter, nil)
			if err != nil {
				return vm.Undefined, err
			}
			if !isObjectValue(res) {
				return vm.Undefined, vmi.NewTypeError("Iterator result is not an object")
			}
			done, err := vmi.GetProperty(res, "done")
			if err != nil {
				return vm.Undefined, err
			}
			if done.IsTruthy() {
				if err := setLengthOrThrow(vmi, a, k); err != nil {
					return vm.Undefined, err
				}
				return a, nil
			}
			v, err := vmi.GetProperty(res, "value")
			if err != nil {
				return vm.Undefined, err
			}
			// A throw from mapfn or the define closes the iterator.
			mv, err := mapValue(v, k)
			if err == nil {
				err = createDataPropertyOrThrow(vmi, a, k, mv)
			}
			if err != nil {
				return vm.Undefined, iteratorClose(vmi, iter, err)
			}
		}
	}

	// Array-like path.
	lenVal, err := vmi.GetProperty(items, "length")
	if err != nil {
		return vm.Undefined, err
	}
	n, err := toLengthWithVM(vmi, lenVal)
	if err != nil {
		return vm.Undefined, err
	}
	a, err := vmi.Construct(c, []vm.Value{vm.NumberValue(float64(n))})
	if err != nil {
		return vm.Undefined, err
	}
	for k := 0; k < n; k++ {
		v, err := vmi.GetProperty(items, strconv.Itoa(k))
		if err != nil {
			return vm.Undefined, err
		}
		mv, err := mapValue(v, k)
		if err != nil {
			return vm.Undefined, err
		}
		if err := createDataPropertyOrThrow(vmi, a, k, mv); err != nil {
			return vm.Undefined, err
		}
	}
	if err := setLengthOrThrow(vmi, a, n); err != nil {
		return vm.Undefined, err
	}
	return a, nil
}

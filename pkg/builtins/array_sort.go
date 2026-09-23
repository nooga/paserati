package builtins

import (
	"math"

	"github.com/nooga/paserati/pkg/vm"
)

// sortCompare is SortCompare(x, y) (ECMA-262 23.1.3.30.2) for two values
// that aren't undefined: comparefn's result through ToNumber (NaN is 0), or
// the UTF-16 order of ToString(x) and ToString(y). The result's sign is the
// order.
func sortCompare(vmInstance *vm.VM, comparefn, x, y vm.Value) (float64, error) {
	if comparefn.IsCallable() {
		res, err := vmInstance.CallArgs2(comparefn, vm.Undefined, x, y)
		if err != nil {
			return 0, err
		}
		n, err := toNumberWithVM(vmInstance, res)
		if err != nil {
			return 0, err
		}
		if math.IsNaN(n) {
			return 0, nil
		}
		return n, nil
	}
	xs, err := getStringValueWithVM(vmInstance, x)
	if err != nil {
		return 0, err
	}
	ys, err := getStringValueWithVM(vmInstance, y)
	if err != nil {
		return 0, err
	}
	return float64(vm.CompareStringsUTF16(xs, ys)), nil
}

// sortValues is the sorting half of SortIndexedProperties: a stable sort of
// items by SortCompare, with every undefined placed last without calling
// comparefn. A throwing comparefn or ToString aborts the sort.
func sortValues(vmInstance *vm.VM, items []vm.Value, comparefn vm.Value) ([]vm.Value, error) {
	defined := make([]vm.Value, 0, len(items))
	undefinedCount := 0
	for _, v := range items {
		if v.IsUndefined() {
			undefinedCount++
		} else {
			defined = append(defined, v)
		}
	}
	// Bottom-up merge sort: stable, O(n log n) comparisons, and each
	// comparison's error can stop it.
	src, dst := defined, make([]vm.Value, len(defined))
	for width := 1; width < len(src); width *= 2 {
		for lo := 0; lo < len(src); lo += 2 * width {
			mid := min(lo+width, len(src))
			hi := min(lo+2*width, len(src))
			i, j, k := lo, mid, lo
			for i < mid && j < hi {
				c, err := sortCompare(vmInstance, comparefn, src[i], src[j])
				if err != nil {
					return nil, err
				}
				if c > 0 {
					dst[k] = src[j]
					j++
				} else {
					dst[k] = src[i]
					i++
				}
				k++
			}
			k += copy(dst[k:], src[i:mid])
			copy(dst[k:], src[j:hi])
		}
		src, dst = dst, src
	}
	for ; undefinedCount > 0; undefinedCount-- {
		src = append(src, vm.Undefined)
	}
	return src, nil
}

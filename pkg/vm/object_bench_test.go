package vm

import (
	"fmt"
	"runtime"
	"testing"
)

// buildObject creates a PlainObject with fieldCount string-keyed own properties
// named field0..field{N-1}, each holding an integer value.
func buildObject(fieldCount int) *PlainObject {
	obj := NewObject(DefaultObjectPrototype).AsPlainObject()
	for i := 0; i < fieldCount; i++ {
		obj.SetOwn(fmt.Sprintf("field%d", i), IntegerValue(int32(i)))
	}
	return obj
}

// benchGetOwn measures cold-path PlainObject.GetOwn: directly call the shape
// scan, bypassing inline caches entirely. This is the path taken by builtins,
// reflection, IC misses, polymorphic sites, and Object.keys-style traversal.
func benchGetOwn(b *testing.B, fieldCount int, accessPattern string) {
	obj := buildObject(fieldCount)
	names := make([]string, fieldCount)
	for i := 0; i < fieldCount; i++ {
		names[i] = fmt.Sprintf("field%d", i)
	}

	var lookup func(i int) string
	switch accessPattern {
	case "first":
		lookup = func(int) string { return names[0] }
	case "last":
		lookup = func(int) string { return names[fieldCount-1] }
	case "round-robin":
		lookup = func(i int) string { return names[i%fieldCount] }
	default:
		b.Fatalf("unknown access pattern %q", accessPattern)
	}

	b.ReportAllocs()
	var sink Value
	var found bool
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, found = obj.GetOwn(lookup(i))
	}
	// (*PlainObject).GetOwn costs 179 against an inline budget of 80, so the call
	// survives today — but (*ArrayObject).GetOwn sits at 84 against that same
	// budget, having crossed it from 73 while this benchmark was in use. Discarding
	// both results is what makes the difference load-bearing: once the callee
	// inlines, nothing observes the work and the loop can be optimised away.
	runtime.KeepAlive(sink)
	runtime.KeepAlive(found)
}

func BenchmarkGetOwn(b *testing.B) {
	sizes := []int{1, 4, 8, 16, 32, 64}
	patterns := []string{"first", "last", "round-robin"}
	for _, n := range sizes {
		for _, p := range patterns {
			b.Run(fmt.Sprintf("n=%d/%s", n, p), func(b *testing.B) {
				benchGetOwn(b, n, p)
			})
		}
	}
}

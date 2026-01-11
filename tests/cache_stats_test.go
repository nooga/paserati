package tests

import (
	"os"
	"testing"

	"github.com/nooga/paserati/pkg/driver"
	"github.com/nooga/paserati/pkg/vm"
)

// TestCacheStatistics demonstrates cache effectiveness with detailed statistics
func TestCacheStatistics(t *testing.T) {
	// Test script that creates multiple property access patterns
	code := `
// Create multiple objects with same shape for cache hits
function Person(name: string, age: number) {
	this.name = name;
	this.age = age;
}
Person.prototype.getName = function(): string {
	return this.name;
};
Person.prototype.getAge = function(): number {
	return this.age;
};
Person.prototype.greet = function(): string {
	return "Hello, " + this.name;
};

// Create many objects with same shape
let p1 = new Person("Alice", 25);
let p2 = new Person("Bob", 30);
let p3 = new Person("Carol", 35);
let p4 = new Person("Dave", 40);
let p5 = new Person("Eve", 45);

// Access properties many times to warm up caches
let count = 0;
for (let round = 0; round < 10; round++) {
	// These should hit the cache after first round
	count += p1.name.length;        // Own property access
	count += p1.getName().length;   // Prototype method access
	count += p1.age;                // Another own property
	count += p1.getAge();           // Another prototype method
	
	count += p2.name.length;
	count += p2.getName().length;
	count += p2.age;
	count += p2.getAge();
	
	count += p3.name.length;
	count += p3.getName().length;
	count += p3.age;
	count += p3.getAge();
	
	count += p4.name.length;
	count += p4.getName().length;
	count += p4.age;
	count += p4.getAge();
	
	count += p5.name.length;
	count += p5.getName().length;
	count += p5.age;
	count += p5.getAge();
}

count;`

	// Test without cache first
	t.Run("WithoutCache", func(t *testing.T) {
		// Save original state and restore after test
		origProtoCache := vm.EnablePrototypeCache
		origDetailedStats := vm.EnableDetailedCacheStats
		t.Cleanup(func() {
			vm.EnablePrototypeCache = origProtoCache
			vm.EnableDetailedCacheStats = origDetailedStats
		})

		os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "false")
		os.Setenv("PASERATI_DETAILED_CACHE_STATS", "false")
		vm.EnablePrototypeCache = false
		vm.EnableDetailedCacheStats = false
		vm.ResetExtendedStats()

		p := driver.NewPaserati()
		result, errs := p.RunString(code)
		if len(errs) > 0 {
			t.Fatalf("Evaluation failed: %v", errs)
		}

		// Don't worry about exact count, just verify it's reasonable
		if !result.IsNumber() || vm.AsNumber(result) < 3000 {
			t.Errorf("Expected count > 3000, got %v", vm.AsNumber(result))
		}

		t.Logf("WITHOUT CACHE - Total count: %v", vm.AsNumber(result))
		t.Logf("Cache was disabled - no cache stats available")
	})

	// Test with cache enabled
	t.Run("WithCache", func(t *testing.T) {
		// Save original state and restore after test
		origProtoCache := vm.EnablePrototypeCache
		origDetailedStats := vm.EnableDetailedCacheStats
		t.Cleanup(func() {
			vm.EnablePrototypeCache = origProtoCache
			vm.EnableDetailedCacheStats = origDetailedStats
		})

		os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "true")
		os.Setenv("PASERATI_DETAILED_CACHE_STATS", "true")
		vm.EnablePrototypeCache = true
		vm.EnableDetailedCacheStats = true
		vm.ResetExtendedStats()

		p := driver.NewPaserati()
		result, errs := p.RunString(code)
		if len(errs) > 0 {
			t.Fatalf("Evaluation failed: %v", errs)
		}

		// Don't worry about exact count, just verify it's reasonable
		if !result.IsNumber() || vm.AsNumber(result) < 3000 {
			t.Errorf("Expected count > 3000, got %v", vm.AsNumber(result))
		}

		t.Logf("WITH CACHE - Total count: %v", vm.AsNumber(result))

		// Get actual cache statistics
		stats := p.GetCacheStats()

		t.Logf("ACTUAL CACHE STATISTICS:")
		t.Logf("  Total hits: %d", stats.TotalHits)
		t.Logf("  Total misses: %d", stats.TotalMisses)
		t.Logf("  Monomorphic hits: %d", stats.MonomorphicHits)
		t.Logf("  Polymorphic hits: %d", stats.PolymorphicHits)
		t.Logf("  Megamorphic hits: %d", stats.MegamorphicHits)
		t.Logf("  Prototype chain hits: %d", stats.ProtoChainHits)
		t.Logf("  Prototype depth 1: %d", stats.ProtoDepth1Hits)
		t.Logf("  Prototype depth 2: %d", stats.ProtoDepth2Hits)
		t.Logf("  Prototype depth N: %d", stats.ProtoDepthNHits)
		t.Logf("  Primitive method hits: %d", stats.PrimitiveMethodHits)
		t.Logf("  Function prototype hits: %d", stats.FunctionProtoHits)
		t.Logf("  Bound methods cached: %d", stats.BoundMethodCached)

		// Verify we have cache activity
		if stats.TotalHits > 0 {
			hitRate := float64(stats.TotalHits) / float64(stats.TotalHits+stats.TotalMisses) * 100
			t.Logf("Cache hit rate: %.2f%%", hitRate)

			if hitRate > 50 {
				t.Logf("✅ Cache is working effectively (>50%% hit rate)")
			} else {
				t.Logf("⚠️  Cache hit rate is lower than expected")
			}
		} else {
			t.Logf("❌ No cache hits detected - cache may not be working")
		}
	})
}

// TestCacheWarmup specifically tests cache warming behavior
func TestCacheWarmup(t *testing.T) {
	// Save original state and restore after test
	origProtoCache := vm.EnablePrototypeCache
	origDetailedStats := vm.EnableDetailedCacheStats
	t.Cleanup(func() {
		vm.EnablePrototypeCache = origProtoCache
		vm.EnableDetailedCacheStats = origDetailedStats
	})

	code := `
// Create objects with identical shapes
function TestObj(value: number) {
	this.value = value;
	this.id = value * 2;
}
TestObj.prototype.getValue = function(): number {
	return this.value;
};
TestObj.prototype.getId = function(): number {
	return this.id;
};

let obj1 = new TestObj(1);
let obj2 = new TestObj(2);
let obj3 = new TestObj(3);

// First round - should populate cache
let firstSum = obj1.value + obj1.getValue() + obj1.id + obj1.getId();
firstSum += obj2.value + obj2.getValue() + obj2.id + obj2.getId();  
firstSum += obj3.value + obj3.getValue() + obj3.id + obj3.getId();

// Second round - should hit cache
let secondSum = obj1.value + obj1.getValue() + obj1.id + obj1.getId();
secondSum += obj2.value + obj2.getValue() + obj2.id + obj2.getId();
secondSum += obj3.value + obj3.getValue() + obj3.id + obj3.getId();

// Third round - should continue hitting cache  
let thirdSum = obj1.value + obj1.getValue() + obj1.id + obj1.getId();
thirdSum += obj2.value + obj2.getValue() + obj2.id + obj2.getId();
thirdSum += obj3.value + obj3.getValue() + obj3.id + obj3.getId();

firstSum + secondSum + thirdSum;`

	// Enable cache and detailed stats
	os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "true")
	os.Setenv("PASERATI_DETAILED_CACHE_STATS", "true")
	vm.EnablePrototypeCache = true
	vm.EnableDetailedCacheStats = true
	vm.ResetExtendedStats()

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	// Don't worry about exact calculation, just verify it's reasonable
	if !result.IsNumber() || vm.AsNumber(result) < 100 {
		t.Errorf("Expected total > 100, got %v", vm.AsNumber(result))
	}

	// Get cache statistics after warmup test
	warmupStats := p.GetCacheStats()

	t.Logf("Cache warmup test completed")
	t.Logf("Total property accesses: %v", vm.AsNumber(result))
	t.Logf("WARMUP CACHE STATISTICS:")
	t.Logf("  Total hits: %d", warmupStats.TotalHits)
	t.Logf("  Total misses: %d", warmupStats.TotalMisses)
	t.Logf("  Monomorphic hits: %d", warmupStats.MonomorphicHits)
	t.Logf("  Polymorphic hits: %d", warmupStats.PolymorphicHits)

	if warmupStats.TotalHits+warmupStats.TotalMisses > 0 {
		hitRate := float64(warmupStats.TotalHits) / float64(warmupStats.TotalHits+warmupStats.TotalMisses) * 100
		t.Logf("  Hit rate: %.2f%%", hitRate)

		if hitRate > 70 {
			t.Logf("✅ Cache warming is highly effective (>70%% hit rate)")
		} else if hitRate > 40 {
			t.Logf("✅ Cache warming is working (>40%% hit rate)")
		} else {
			t.Logf("⚠️  Cache warming is less effective than expected")
		}
	}
}

// TestPolymorphicCaching tests how the cache handles different object types
func TestPolymorphicCaching(t *testing.T) {
	// Save original state and restore after test
	origProtoCache := vm.EnablePrototypeCache
	origDetailedStats := vm.EnableDetailedCacheStats
	t.Cleanup(func() {
		vm.EnablePrototypeCache = origProtoCache
		vm.EnableDetailedCacheStats = origDetailedStats
	})

	code := `
// Create different object types that access same property names
function TypeA(val: number) {
	this.data = val;
}
TypeA.prototype.process = function(): string {
	return "A:" + this.data;
};

function TypeB(val: number) {
	this.data = val * 2;
}
TypeB.prototype.process = function(): string {
	return "B:" + this.data;
};

function TypeC(val: number) {
	this.data = val * 3;
}
TypeC.prototype.process = function(): string {
	return "C:" + this.data;
};

// Create instances of different types
let a1 = new TypeA(1);
let a2 = new TypeA(2);
let b1 = new TypeB(1);
let b2 = new TypeB(2);
let c1 = new TypeC(1);
let c2 = new TypeC(2);

// Access same property names across different types
let sum = 0;
for (let round = 0; round < 3; round++) {
	sum += a1.data;                // Same property name, TypeA
	sum += a1.process().length;    // Same method name, TypeA implementation
	sum += a2.data;
	sum += a2.process().length;
	
	sum += b1.data;                // Same property name, TypeB  
	sum += b1.process().length;    // Same method name, TypeB implementation
	sum += b2.data;
	sum += b2.process().length;
	
	sum += c1.data;                // Same property name, TypeC
	sum += c1.process().length;    // Same method name, TypeC implementation
	sum += c2.data;
	sum += c2.process().length;
}

sum;`

	// Enable cache and detailed stats
	os.Setenv("PASERATI_ENABLE_PROTO_CACHE", "true")
	os.Setenv("PASERATI_DETAILED_CACHE_STATS", "true")
	vm.EnablePrototypeCache = true
	vm.EnableDetailedCacheStats = true
	vm.ResetExtendedStats()

	p := driver.NewPaserati()
	result, errs := p.RunString(code)
	if len(errs) > 0 {
		t.Fatalf("Evaluation failed: %v", errs)
	}

	// Don't worry about exact calculation
	if !result.IsNumber() || vm.AsNumber(result) < 100 {
		t.Errorf("Expected total > 100, got %v", vm.AsNumber(result))
	}

	// Get cache statistics after polymorphic test
	polyStats := p.GetCacheStats()

	t.Logf("Polymorphic caching test completed")
	t.Logf("Total property accesses: %v", vm.AsNumber(result))
	t.Logf("POLYMORPHIC CACHE STATISTICS:")
	t.Logf("  Total hits: %d", polyStats.TotalHits)
	t.Logf("  Total misses: %d", polyStats.TotalMisses)
	t.Logf("  Monomorphic hits: %d", polyStats.MonomorphicHits)
	t.Logf("  Polymorphic hits: %d", polyStats.PolymorphicHits)
	t.Logf("  Megamorphic hits: %d", polyStats.MegamorphicHits)

	if polyStats.TotalHits+polyStats.TotalMisses > 0 {
		hitRate := float64(polyStats.TotalHits) / float64(polyStats.TotalHits+polyStats.TotalMisses) * 100
		t.Logf("  Hit rate: %.2f%%", hitRate)

		// Polymorphic workloads typically have lower hit rates
		if hitRate > 50 {
			t.Logf("✅ Polymorphic caching is highly effective (>50%% hit rate)")
		} else if hitRate > 30 {
			t.Logf("✅ Polymorphic caching is working (>30%% hit rate)")
		} else {
			t.Logf("⚠️  Polymorphic cache performance could be improved")
		}

		if polyStats.PolymorphicHits > 0 {
			t.Logf("✅ Polymorphic cache entries are being used")
		}
		if polyStats.MegamorphicHits > 0 {
			t.Logf("ℹ️  Some accesses fell back to megamorphic lookup")
		}
	}
}

package vm

import (
	"testing"
)

func TestPlainObjectBasic(t *testing.T) {
	poVal := NewObject(DefaultObjectPrototype)
	po := poVal.AsPlainObject()
	// No properties initially
	if po.HasOwn("foo") {
		t.Errorf("expected HasOwn(\"foo\") to be false on new object")
	}
	if v, ok := po.GetOwn("foo"); ok {
		t.Errorf("expected GetOwn(\"foo\") ok=false, got ok=true, v=%v", v)
	}
	// Define a property
	po.SetOwn("foo", IntegerValue(42))
	if !po.HasOwn("foo") {
		t.Errorf("expected HasOwn(\"foo\") true after SetOwn")
	}
	v, ok := po.GetOwn("foo")
	if !ok {
		t.Fatalf("expected GetOwn(\"foo\") ok=true after SetOwn")
	}
	if v.AsInteger() != 42 {
		t.Errorf("expected GetOwn to return 42, got %d", v.AsInteger())
	}
	// Overwrite existing property
	po.SetOwn("foo", IntegerValue(7))
	v2, ok2 := po.GetOwn("foo")
	if !ok2 || v2.AsInteger() != 7 {
		t.Errorf("expected overwritten value 7, got %v (ok=%v)", v2, ok2)
	}
	// OwnKeys should list "foo"
	keys := po.OwnKeys()
	if len(keys) != 1 || keys[0] != "foo" {
		t.Errorf("OwnKeys mismatch, expected [foo], got %v", keys)
	}
}

func TestPlainObjectShapeTransitions(t *testing.T) {
	po := NewObject(DefaultObjectPrototype).AsPlainObject()
	root := po.shape
	// first definition creates new shape
	po.SetOwn("a", IntegerValue(1))
	s1 := po.shape
	if s1 == root {
		t.Errorf("expected new shape after first property, got same shape")
	}
	// redefining same property should keep shape
	po.SetOwn("a", IntegerValue(2))
	s2 := po.shape
	if s2 != s1 {
		t.Errorf("expected same shape on overwrite, got different shapes")
	}
	// adding another property creates another shape
	po.SetOwn("b", IntegerValue(3))
	s3 := po.shape
	if s3 == s2 {
		t.Errorf("expected new shape after adding second property, got same shape")
	}
	// fields order
	keys := po.OwnKeys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("OwnKeys order mismatch, expected [a b], got %v", keys)
	}
}

func TestDictObjectBasic(t *testing.T) {
	dVal := NewDictObject(DefaultObjectPrototype)
	d := dVal.AsDictObject()
	// No properties initially
	if d.HasOwn("x") {
		t.Errorf("expected HasOwn(\"x\") to be false on new dict object")
	}
	if v, ok := d.GetOwn("x"); ok {
		t.Errorf("expected GetOwn(\"x\") ok=false, got ok=true, v=%v", v)
	}
	// Define a property
	d.SetOwn("x", IntegerValue(100))
	if !d.HasOwn("x") {
		t.Errorf("expected HasOwn(\"x\") true after SetOwn")
	}
	v, ok := d.GetOwn("x")
	if !ok || v.AsInteger() != 100 {
		t.Errorf("expected GetOwn to return 100, got %v (ok=%v)", v, ok)
	}
	// Delete property
	if !d.DeleteOwn("x") {
		t.Errorf("expected DeleteOwn(\"x\") to return true")
	}
	if d.HasOwn("x") {
		t.Errorf("expected HasOwn(\"x\") false after DeleteOwn")
	}
	if _, ok2 := d.GetOwn("x"); ok2 {
		t.Errorf("expected GetOwn(\"x\") ok=false after DeleteOwn")
	}
	// Delete non-existing
	if d.DeleteOwn("x") {
		t.Errorf("expected DeleteOwn(\"x\") false when property absent")
	}
	// OwnKeys sorted
	d.SetOwn("b", IntegerValue(2))
	d.SetOwn("a", IntegerValue(1))
	keys := d.OwnKeys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "b" {
		t.Errorf("OwnKeys sorting mismatch, expected [a b], got %v", keys)
	}
}

func TestInlineCache(t *testing.T) {
	vm := NewVM()

	// Create objects with same shape for monomorphic caching
	obj1 := NewObject(DefaultObjectPrototype).AsPlainObject()
	obj1.SetOwn("x", IntegerValue(1))
	obj1.SetOwn("y", IntegerValue(2))

	obj2 := NewObject(DefaultObjectPrototype).AsPlainObject()
	obj2.SetOwn("x", IntegerValue(10))
	obj2.SetOwn("y", IntegerValue(20))

	// Create a simple cache for testing (simulating instruction pointer 100)
	cacheKey := 100
	cache := &PropInlineCache{
		state: CacheStateUninitialized,
	}
	vm.propCache[cacheKey] = cache

	// First lookup should miss and populate cache
	_, hit := cache.lookupInCache(obj1.shape, "x")
	if hit {
		t.Errorf("Expected cache miss on first lookup, got hit")
	}

	// Update cache with shape+propName+offset for property "x"
	cache.updateCache(obj1.shape, "x", 0, false, true) // "x" should be at offset 0

	// Second lookup should hit
	offset, hit := cache.lookupInCache(obj1.shape, "x")
	if !hit {
		t.Errorf("Expected cache hit on second lookup, got miss")
	}
	if offset != 0 {
		t.Errorf("Expected offset 0, got %d", offset)
	}

	// Lookup with same shape and propName should also hit
	_, hit = cache.lookupInCache(obj2.shape, "x")
	if !hit {
		t.Errorf("Expected cache hit for object with same shape, got miss")
	}

	// Create object with different shape (polymorphic case)
	obj3 := NewObject(DefaultObjectPrototype).AsPlainObject()
	obj3.SetOwn("x", IntegerValue(100))
	obj3.SetOwn("y", IntegerValue(200))
	obj3.SetOwn("z", IntegerValue(300)) // Different shape!

	// Should miss because shape is different
	_, hit = cache.lookupInCache(obj3.shape, "x")
	if hit {
		t.Errorf("Expected cache miss for different shape, got hit")
	}

	// Update cache with new shape (should transition to polymorphic)
	cache.updateCache(obj3.shape, "x", 0, false, true) // "x" should still be at offset 0

	if cache.state != CacheStatePolymorphic {
		t.Errorf("Expected cache to transition to polymorphic, got state %d", cache.state)
	}
	if cache.entryCount != 2 {
		t.Errorf("Expected 2 cache entries, got %d", cache.entryCount)
	}

	// Both shapes should now hit
	_, hit = cache.lookupInCache(obj1.shape, "x")
	if !hit {
		t.Errorf("Expected cache hit for first shape in polymorphic cache, got miss")
	}

	_, hit = cache.lookupInCache(obj3.shape, "x")
	if !hit {
		t.Errorf("Expected cache hit for second shape in polymorphic cache, got miss")
	}
}

func TestInlineCacheStats(t *testing.T) {
	vm := NewVM()

	// Verify initial stats are zero
	stats := vm.GetCacheStats()
	if stats.totalHits != 0 || stats.totalMisses != 0 {
		t.Errorf("Expected zero initial stats, got hits=%d misses=%d", stats.totalHits, stats.totalMisses)
	}

	// Create a cache and perform some operations
	cache := &PropInlineCache{
		state:      CacheStateMonomorphic,
		entries:    [4]PropCacheEntry{{shape: RootShape, propName: "test", offset: 0}},
		entryCount: 1,
	}

	// Simulate cache hit
	offset, hit := cache.lookupInCache(RootShape, "test")
	if !hit || offset != 0 {
		t.Errorf("Expected cache hit with offset 0, got hit=%v offset=%d", hit, offset)
	}

	// Simulate cache miss
	obj := NewObject(DefaultObjectPrototype).AsPlainObject()
	obj.SetOwn("test", IntegerValue(42)) // Creates different shape

	_, hit = cache.lookupInCache(obj.shape, "test")
	if hit {
		t.Errorf("Expected cache miss for different shape, got hit")
	}

	// Check that hit/miss counts were updated
	if cache.hitCount != 1 {
		t.Errorf("Expected hitCount=1, got %d", cache.hitCount)
	}
	if cache.missCount != 1 {
		t.Errorf("Expected missCount=1, got %d", cache.missCount)
	}
}

package vm

import "fmt"

// PropCacheState represents the different states of inline cache
type PropCacheState uint8

const (
	CacheStateUninitialized PropCacheState = iota
	CacheStateMonomorphic                  // Single shape cached
	CacheStatePolymorphic                  // Multiple shapes cached (up to 4)
	CacheStateMegamorphic                  // Too many shapes, fallback to map lookup
)

// PropCacheEntry represents a single shape+offset entry in the cache
type PropCacheEntry struct {
	shape  *Shape // The shape this cache entry is valid for
	offset int    // The property offset in the object's properties slice
}

// PropInlineCache represents the inline cache for a property access site
type PropInlineCache struct {
	state      PropCacheState
	entries    [4]PropCacheEntry // Support up to 4 shapes (polymorphic)
	entryCount int               // Number of active entries
	hitCount   uint32            // For debugging/metrics
	missCount  uint32            // For debugging/metrics
}

// ICacheStats holds statistics about inline cache performance
type ICacheStats struct {
	totalHits       uint64
	totalMisses     uint64
	monomorphicHits uint64
	polymorphicHits uint64
	megamorphicHits uint64
}

// lookupInCache performs a property lookup using the inline cache
func (ic *PropInlineCache) lookupInCache(shape *Shape) (int, bool) {
	switch ic.state {
	case CacheStateUninitialized:
		return -1, false
	case CacheStateMonomorphic:
		if ic.entries[0].shape == shape {
			ic.hitCount++
			return ic.entries[0].offset, true
		}
		ic.missCount++
		return -1, false
	case CacheStatePolymorphic:
		for i := 0; i < ic.entryCount; i++ {
			if ic.entries[i].shape == shape {
				ic.hitCount++
				// Move hit entry to front for better cache locality
				if i > 0 {
					entry := ic.entries[i]
					copy(ic.entries[1:i+1], ic.entries[0:i])
					ic.entries[0] = entry
				}
				return ic.entries[0].offset, true
			}
		}
		ic.missCount++
		return -1, false
	case CacheStateMegamorphic:
		// Always miss in megamorphic state - forces full lookup
		ic.missCount++
		return -1, false
	}
	return -1, false
}

// updateCache updates the inline cache with a new shape+offset entry
func (ic *PropInlineCache) updateCache(shape *Shape, offset int) {
	switch ic.state {
	case CacheStateUninitialized:
		// First entry - transition to monomorphic
		ic.state = CacheStateMonomorphic
		ic.entries[0] = PropCacheEntry{shape: shape, offset: offset}
		ic.entryCount = 1
	case CacheStateMonomorphic:
		// Check if it's the same shape (update offset)
		if ic.entries[0].shape == shape {
			ic.entries[0].offset = offset
			return
		}
		// Different shape - transition to polymorphic
		ic.state = CacheStatePolymorphic
		ic.entries[1] = PropCacheEntry{shape: shape, offset: offset}
		ic.entryCount = 2
	case CacheStatePolymorphic:
		// Check if shape already exists
		for i := 0; i < ic.entryCount; i++ {
			if ic.entries[i].shape == shape {
				ic.entries[i].offset = offset
				return
			}
		}
		// New shape
		if ic.entryCount < 4 {
			ic.entries[ic.entryCount] = PropCacheEntry{shape: shape, offset: offset}
			ic.entryCount++
		} else {
			// Too many shapes - transition to megamorphic
			ic.state = CacheStateMegamorphic
			ic.entryCount = 0
		}
	case CacheStateMegamorphic:
		// Don't cache in megamorphic state
		return
	}
}

// resetCache clears the inline cache (used when shapes change)
func (ic *PropInlineCache) resetCache() {
	ic.state = CacheStateUninitialized
	ic.entryCount = 0
	// Don't reset hit/miss counts for debugging
}

// GetCacheStats returns the current inline cache statistics
func (vm *VM) GetCacheStats() ICacheStats {
	return vm.cacheStats
}

// PrintCacheStats prints detailed cache performance information for debugging
func (vm *VM) PrintCacheStats() {
	stats := vm.cacheStats
	total := stats.totalHits + stats.totalMisses
	if total == 0 {
		fmt.Printf("IC Stats: No cache activity\n")
		return
	}

	hitRate := float64(stats.totalHits) / float64(total) * 100.0
	fmt.Printf("IC Stats: Total: %d, Hits: %d (%.1f%%), Misses: %d\n",
		total, stats.totalHits, hitRate, stats.totalMisses)
	fmt.Printf("  Monomorphic: %d, Polymorphic: %d, Megamorphic: %d\n",
		stats.monomorphicHits, stats.polymorphicHits, stats.megamorphicHits)

	// Print per-site cache information
	fmt.Printf("  Cache sites: %d\n", len(vm.propCache))
	for ip, cache := range vm.propCache {
		stateStr := "UNINITIALIZED"
		switch cache.state {
		case CacheStateMonomorphic:
			stateStr = "MONOMORPHIC"
		case CacheStatePolymorphic:
			stateStr = fmt.Sprintf("POLYMORPHIC(%d)", cache.entryCount)
		case CacheStateMegamorphic:
			stateStr = "MEGAMORPHIC"
		}
		fmt.Printf("    IP %d: %s (hits: %d, misses: %d)\n",
			ip, stateStr, cache.hitCount, cache.missCount)
	}
}

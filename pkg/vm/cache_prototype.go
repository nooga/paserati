package vm

import (
	"fmt"
	"os"
	"strconv"
)

// Cache configuration flags - can be set via environment variables
var (
	// EnablePrototypeCache enables caching of prototype chain lookups
	EnablePrototypeCache = getEnvBool("PASERATI_ENABLE_PROTO_CACHE", false)

	// EnableDetailedCacheStats enables collection of detailed per-site statistics
	EnableDetailedCacheStats = getEnvBool("PASERATI_DETAILED_CACHE_STATS", false)

	// MaxPolymorphicEntries controls how many shapes we track before going megamorphic
	MaxPolymorphicEntries = getEnvInt("PASERATI_MAX_POLY_ENTRIES", 4)
)

// getEnvBool reads a boolean environment variable with a default value
func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}

// getEnvInt reads an integer environment variable with a default value
func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// generateCacheKey creates a unique key for a property access site
func generateCacheKey(ip int, propName string) int {
	propNameHash := 0
	for _, b := range []byte(propName) {
		propNameHash = propNameHash*31 + int(b)
	}
	// Adjust for the fact that ip has been advanced
	return (ip-5)*100000 + (propNameHash & 0xFFFF)
}

// PrototypeCacheEntry represents a cached prototype chain lookup
type PrototypeCacheEntry struct {
	objectShape    *Shape       // Shape of the object being accessed
	prototypeObj   *PlainObject // Prototype where property was found (nil if not found)
	prototypeDepth int          // How many steps up the chain (0=own, 1=proto, 2=proto.proto)
	offset         int          // Property offset in the prototype object
	boundMethod    Value        // Cached bound method (for primitives)
	isMethod       bool         // Whether this is a method requiring 'this' binding
}

// PrototypeCache holds prototype chain cache entries for a property access site
type PrototypeCache struct {
	entries    [4]PrototypeCacheEntry
	entryCount int
}

// VM extension for prototype caching (stored separately from main propCache)
var prototypeCache map[int]*PrototypeCache

func init() {
	if EnablePrototypeCache {
		prototypeCache = make(map[int]*PrototypeCache)
	}
}

// GetOrCreatePrototypeCache gets or creates a prototype cache for the given key
func GetOrCreatePrototypeCache(cacheKey int) *PrototypeCache {
	if !EnablePrototypeCache {
		return nil
	}

	// Initialize the map if it's nil
	if prototypeCache == nil {
		prototypeCache = make(map[int]*PrototypeCache)
	}

	if cache, exists := prototypeCache[cacheKey]; exists {
		return cache
	}

	cache := &PrototypeCache{}
	prototypeCache[cacheKey] = cache
	return cache
}

// Lookup checks the prototype chain cache
func (pc *PrototypeCache) Lookup(shape *Shape) (*PrototypeCacheEntry, bool) {
	if pc == nil || pc.entryCount == 0 {
		return nil, false
	}

	for i := 0; i < pc.entryCount; i++ {
		if pc.entries[i].objectShape == shape {
			// Move to front for better cache locality
			if i > 0 {
				entry := pc.entries[i]
				copy(pc.entries[1:i+1], pc.entries[0:i])
				pc.entries[0] = entry
			}
			return &pc.entries[0], true
		}
	}

	return nil, false
}

// Update adds or updates a prototype chain cache entry
func (pc *PrototypeCache) Update(shape *Shape, protoObj *PlainObject, depth int, offset int, boundMethod Value, isMethod bool) {
	if pc == nil {
		return
	}

	entry := PrototypeCacheEntry{
		objectShape:    shape,
		prototypeObj:   protoObj,
		prototypeDepth: depth,
		offset:         offset,
		boundMethod:    boundMethod,
		isMethod:       isMethod,
	}

	// Check if shape already exists
	for i := 0; i < pc.entryCount; i++ {
		if pc.entries[i].objectShape == shape {
			pc.entries[i] = entry
			return
		}
	}

	// Add new entry
	if pc.entryCount < len(pc.entries) {
		pc.entries[pc.entryCount] = entry
		pc.entryCount++
	} else {
		// Evict oldest (last) entry
		copy(pc.entries[1:], pc.entries[0:len(pc.entries)-1])
		pc.entries[0] = entry
	}
}

// ExtendedCacheStats includes prototype-specific statistics
type ExtendedCacheStats struct {
	// Basic stats
	TotalHits       uint64
	TotalMisses     uint64
	MonomorphicHits uint64
	PolymorphicHits uint64
	MegamorphicHits uint64

	// Prototype chain stats
	ProtoChainHits   uint64
	ProtoChainMisses uint64
	ProtoDepth1Hits  uint64 // Direct prototype hits
	ProtoDepth2Hits  uint64 // Prototype.prototype hits
	ProtoDepthNHits  uint64 // Deeper prototype hits

	// Detailed stats (when EnableDetailedCacheStats is true)
	PrimitiveMethodHits uint64 // String.prototype, Array.prototype method hits
	FunctionProtoHits   uint64 // Function.prototype method hits
	BoundMethodCached   uint64 // Number of bound methods cached
}

// Global extended stats (alongside the basic VM stats)
var extendedStats ExtendedCacheStats

// UpdatePrototypeStats updates prototype-specific statistics
func UpdatePrototypeStats(statType string, depth int) {
	if !EnableDetailedCacheStats {
		return
	}

	switch statType {
	case "proto_hit":
		extendedStats.ProtoChainHits++
		switch depth {
		case 1:
			extendedStats.ProtoDepth1Hits++
		case 2:
			extendedStats.ProtoDepth2Hits++
		default:
			if depth > 2 {
				extendedStats.ProtoDepthNHits++
			}
		}
	case "proto_miss":
		extendedStats.ProtoChainMisses++
	case "primitive_method":
		extendedStats.PrimitiveMethodHits++
	case "function_proto":
		extendedStats.FunctionProtoHits++
	case "bound_method_cached":
		extendedStats.BoundMethodCached++
	}
}

// CopyBasicStats copies basic cache stats from VM to extended stats
func CopyBasicStats(vmStats ICacheStats) {
	extendedStats.TotalHits = vmStats.totalHits
	extendedStats.TotalMisses = vmStats.totalMisses
	extendedStats.MonomorphicHits = vmStats.monomorphicHits
	extendedStats.PolymorphicHits = vmStats.polymorphicHits
	extendedStats.MegamorphicHits = vmStats.megamorphicHits
}

// PrintExtendedStats prints detailed cache statistics including prototype chain info
func PrintExtendedStats(vm *VM) {
	// Copy basic stats first
	CopyBasicStats(vm.cacheStats)

	// Print basic stats using existing function
	vm.PrintCacheStats()

	// Print prototype chain stats if enabled
	if EnablePrototypeCache {
		fmt.Printf("\nPrototype Chain Cache Stats:\n")
		protoTotal := extendedStats.ProtoChainHits + extendedStats.ProtoChainMisses
		if protoTotal > 0 {
			protoHitRate := float64(extendedStats.ProtoChainHits) / float64(protoTotal) * 100.0
			fmt.Printf("  Total: %d, Hits: %d (%.1f%%), Misses: %d\n",
				protoTotal, extendedStats.ProtoChainHits, protoHitRate, extendedStats.ProtoChainMisses)
			fmt.Printf("  Depth Distribution - Direct: %d, Proto: %d, Deep: %d\n",
				extendedStats.ProtoDepth1Hits, extendedStats.ProtoDepth2Hits, extendedStats.ProtoDepthNHits)
		} else {
			fmt.Printf("  No prototype chain cache activity\n")
		}

		fmt.Printf("  Cache sites: %d\n", len(prototypeCache))
	}

	// Print detailed stats if enabled
	if EnableDetailedCacheStats {
		fmt.Printf("\nDetailed Cache Stats:\n")
		fmt.Printf("  Primitive method hits: %d\n", extendedStats.PrimitiveMethodHits)
		fmt.Printf("  Function prototype hits: %d\n", extendedStats.FunctionProtoHits)
		fmt.Printf("  Bound methods cached: %d\n", extendedStats.BoundMethodCached)
	}

	// Print configuration
	fmt.Printf("\nCache Configuration:\n")
	fmt.Printf("  Prototype caching: %v\n", EnablePrototypeCache)
	fmt.Printf("  Detailed stats: %v\n", EnableDetailedCacheStats)
	fmt.Printf("  Max polymorphic entries: %d\n", MaxPolymorphicEntries)
}

// ResetExtendedStats resets all extended statistics
func ResetExtendedStats() {
	extendedStats = ExtendedCacheStats{}
	if EnablePrototypeCache {
		prototypeCache = make(map[int]*PrototypeCache)
	}
}

// GetExtendedStats returns a copy of the current extended cache statistics
// Note: This only returns the extended prototype stats, not the VM's basic cache stats
func GetExtendedStats() ExtendedCacheStats {
	return extendedStats
}

// GetExtendedStatsFromVM returns extended stats combined with VM's basic cache stats
func GetExtendedStatsFromVM(vm *VM) ExtendedCacheStats {
	// Copy basic stats from VM
	CopyBasicStats(vm.cacheStats)
	return extendedStats
}

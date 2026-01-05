package vm

// getOrCreatePropInlineCache returns the inline cache for a specific property access site
// in the current chunk, indexed by opcode start offset.
//
// This replaces the older global map-based cache for the hot OpGetProp/OpSetProp path,
// avoiding a Go map lookup and cache-key hashing on every property access.
func (vm *VM) getOrCreatePropInlineCache(frame *CallFrame, siteIP int) *PropInlineCache {
	if frame == nil || frame.closure == nil || frame.closure.Fn == nil || frame.closure.Fn.Chunk == nil {
		// Fallback: no stable per-chunk location, return a throwaway cache.
		return &PropInlineCache{state: CacheStateUninitialized}
	}
	chunk := frame.closure.Fn.Chunk
	if siteIP < 0 || siteIP >= len(chunk.Code) {
		return &PropInlineCache{state: CacheStateUninitialized}
	}

	// Lazily allocate cache table sized to bytecode.
	if chunk.propInlineCaches == nil || len(chunk.propInlineCaches) != len(chunk.Code) {
		chunk.propInlineCaches = make([]*PropInlineCache, len(chunk.Code))
	}
	ic := chunk.propInlineCaches[siteIP]
	if ic == nil {
		ic = &PropInlineCache{state: CacheStateUninitialized}
		chunk.propInlineCaches[siteIP] = ic
	}
	return ic
}



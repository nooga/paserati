package vm

import (
	"math"
	"sort"
	"sync/atomic"
	"unsafe"
)

// JavaScript indexes strings by UTF-16 code unit; Paserati stores them as
// Go (WTF-8) strings. Every code-unit index is therefore a decode, and the
// naive implementation re-decoded the whole string on every charAt /
// charCodeAt / codePointAt / .length. TypeScript's scanner is
// `text.charCodeAt(pos)` in a tight loop, so a few-thousand-line .d.ts was
// re-encoded thousands of times: O(n^2) CPU plus the GC churn of a fresh
// []uint16 per call.
//
// Two things fix that:
//
//  1. ASCII fast path. An all-ASCII string (the overwhelming common case -
//     every lib.*.d.ts and @types/node .d.ts is pure ASCII) has .length ==
//     len(s) and code unit i == s[i], with no allocation and no decode.
//
//  2. A classification cache. IsASCII is still an O(n) scan, so a scanner
//     looping charCodeAt over one string would stay O(n^2) without it. The
//     cache memoises, per string, whether it is ASCII and (if not) its
//     materialised []uint16 view, so a forward scan pays O(n) once.
//
// The cache is a small direct-mapped table of immutable entries behind
// atomic.Pointer slots. It is keyed by string identity - the (data pointer,
// length) header, not the bytes - so lookup is unconditionally O(1) and can
// never return a wrong answer: Go strings are immutable and a reachable entry
// pins the backing array, so a (ptr,len) match implies identical bytes. Races
// on a slot are benign (worst case: a redundant re-classification). This keeps
// the exported helpers usable from every call site - builtins, property
// helpers, the interpreter loop - without threading *VM through all of them.

const utf16CacheSlots = 8 // must be a power of two

type utf16Entry struct {
	ptr     *byte    // identity: data pointer of the classified string
	byteLen int      // identity: len(s) in bytes
	n       int      // length in UTF-16 code units
	ascii   bool     // all code points < 0x80
	units   []uint16 // materialised view; nil when ascii
	// byteOff[u] is the byte offset of code unit u, with byteOff[n] == len(s).
	// nil when ascii, where the mapping is the identity. Non-decreasing, so a
	// byte offset can be mapped back with a binary search.
	byteOff []int32
}

var utf16Cache [utf16CacheSlots]atomic.Pointer[utf16Entry]

var emptyUTF16Entry = &utf16Entry{ptr: nil, n: 0, ascii: true}

// stringInfo returns the (cached) UTF-16 classification of s. It never returns
// nil. On a miss it classifies s - and, for non-ASCII strings, materialises the
// full []uint16 view so that n is exactly len(StringToUTF16(s)) by construction
// - then stores the entry for next time.
func stringInfo(s string) *utf16Entry {
	if len(s) == 0 {
		return emptyUTF16Entry
	}
	ptr := unsafe.StringData(s)
	slot := (uintptr(unsafe.Pointer(ptr)) ^ uintptr(len(s))) >> 3 & (utf16CacheSlots - 1)
	if e := utf16Cache[slot].Load(); e != nil && e.ptr == ptr && e.byteLen == len(s) {
		return e
	}

	var e *utf16Entry
	if IsASCII(s) {
		e = &utf16Entry{ptr: ptr, byteLen: len(s), n: len(s), ascii: true}
	} else {
		units, offs := stringToUTF16WithOffsets(s, len(s) <= math.MaxInt32)
		e = &utf16Entry{ptr: ptr, byteLen: len(s), n: len(units), ascii: false,
			units: units, byteOff: offs}
	}
	utf16Cache[slot].Store(e)
	return e
}

// IsASCII reports whether every byte of s is < 0x80, i.e. the string is a
// sequence of one-byte UTF-16 code units. Scans 8 bytes at a time.
func IsASCII(s string) bool {
	n := len(s)
	if n == 0 {
		return true
	}
	const highBits = uint64(0x8080808080808080)
	ptr := unsafe.Pointer(unsafe.StringData(s))
	i := 0
	for ; i+8 <= n; i += 8 {
		chunk := *(*uint64)(unsafe.Pointer(uintptr(ptr) + uintptr(i)))
		if chunk&highBits != 0 {
			return false
		}
	}
	for ; i < n; i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// UTF16Length returns the number of UTF-16 code units in s - the value
// JavaScript's String.prototype.length reports.
func UTF16Length(s string) int {
	return stringInfo(s).n
}

// UTF16CodeUnitAt returns the UTF-16 code unit at index idx and whether idx was
// in range. This is the primitive behind charCodeAt and indexed string access.
func UTF16CodeUnitAt(s string, idx int) (uint16, bool) {
	if idx < 0 {
		return 0, false
	}
	e := stringInfo(s)
	if idx >= e.n {
		return 0, false
	}
	if e.ascii {
		return uint16(s[idx]), true
	}
	return e.units[idx], true
}

// UTF16CodePointAt returns the code point at UTF-16 index idx, combining a
// surrogate pair when idx points at a lead surrogate followed by a trail
// surrogate (String.prototype.codePointAt semantics), and whether idx was in
// range.
func UTF16CodePointAt(s string, idx int) (uint32, bool) {
	if idx < 0 {
		return 0, false
	}
	e := stringInfo(s)
	if idx >= e.n {
		return 0, false
	}
	if e.ascii {
		return uint32(s[idx]), true
	}
	first := e.units[idx]
	if first < 0xD800 || first > 0xDBFF || idx+1 >= e.n {
		return uint32(first), true
	}
	second := e.units[idx+1]
	if second < 0xDC00 || second > 0xDFFF {
		return uint32(first), true
	}
	return (uint32(first)-0xD800)*0x400 + (uint32(second) - 0xDC00) + 0x10000, true
}

// UTF16ToByteOffset maps a UTF-16 code unit offset to a byte offset into s,
// clamped to the string. An offset landing between the halves of a surrogate
// pair rounds up to the end of that character: a UTF-8 buffer cannot hold half
// of one, and rounding up keeps the result a valid string boundary.
//
// This is the conversion String.prototype.substring / slice / substr need. It
// used to live in builtins as a fresh `for range s` from byte 0 on every call,
// which made TypeScript's scanner - text.substring(tokenPos, pos) over the whole
// source, once per token - quadratic in file length.
func UTF16ToByteOffset(s string, u16 int) int {
	if u16 <= 0 {
		return 0
	}
	e := stringInfo(s)
	if u16 >= e.n {
		return len(s)
	}
	if e.ascii {
		// One byte per code unit, so the two coordinate systems coincide.
		return u16
	}
	if e.byteOff == nil {
		return utf16ToByteOffsetUncached(s, u16)
	}
	// Both halves of a surrogate pair record their character's start, so a tie
	// with the previous unit means u16 is a trail surrogate. It has no byte
	// position of its own; round up to the end of the character, which is where
	// the next one begins.
	if u16 > 0 && e.byteOff[u16] == e.byteOff[u16-1] {
		return int(e.byteOff[u16+1])
	}
	return int(e.byteOff[u16])
}

// ByteToUTF16Offset maps a byte offset into s to a UTF-16 code unit offset -
// the inverse of UTF16ToByteOffset, and what the regex path needs to report
// match positions in the units ECMAScript counts.
//
// For a byte offset that is not a character boundary the result is the index of
// the unit whose character contains it. Callers pass boundaries in practice:
// the offsets come from the regex engine and from strings.Index, both of which
// return them.
func ByteToUTF16Offset(s string, b int) int {
	if b <= 0 {
		return 0
	}
	e := stringInfo(s)
	if b >= len(s) {
		return e.n
	}
	if e.ascii {
		return b
	}
	if e.byteOff == nil {
		return len(stringToUTF16Uncached(s[:b]))
	}
	// byteOff is non-decreasing, so the first unit at or past b is the answer.
	return sort.Search(e.n, func(u int) bool { return int(e.byteOff[u]) >= b })
}

// utf16ToByteOffsetUncached is the walking fallback, used only when the offset
// index was not built (a string longer than MaxInt32 bytes).
func utf16ToByteOffsetUncached(s string, u16 int) int {
	n := 0
	for i, r := range s {
		if n >= u16 {
			return i
		}
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return len(s)
}

// UTF16CharAt returns the one-code-unit substring at UTF-16 index idx (what
// charAt and String.prototype.at hand back) and whether idx was in range. A
// lone surrogate is re-encoded as WTF-8 via UTF16ToString rather than through
// string(rune(u)), which would replace it with U+FFFD.
func UTF16CharAt(s string, idx int) (string, bool) {
	u, ok := UTF16CodeUnitAt(s, idx)
	if !ok {
		return "", false
	}
	if u < 0x80 {
		return string(rune(u)), true
	}
	return UTF16ToString([]uint16{u}), true
}

// StringToUTF16 converts a Go (WTF-8) string to a freshly allocated slice of
// UTF-16 code units. Lone surrogates produced by the lexer's WTF-8 encoding are
// preserved. The returned slice is owned by the caller. Prefer UTF16Length /
// UTF16CodeUnitAt / UTF16CodePointAt when a full materialisation is not needed.
func StringToUTF16(s string) []uint16 {
	e := stringInfo(s)
	if e.ascii {
		out := make([]uint16, len(s))
		for i := 0; i < len(s); i++ {
			out[i] = uint16(s[i])
		}
		return out
	}
	out := make([]uint16, len(e.units))
	copy(out, e.units)
	return out
}

// stringToUTF16Uncached is the raw WTF-8 -> UTF-16 decoder. It handles WTF-8
// encoded lone surrogates that our lexer produces. Callers should go through
// StringToUTF16 / stringInfo so the result is cached.
func stringToUTF16Uncached(s string) []uint16 {
	units, _ := stringToUTF16WithOffsets(s, false)
	return units
}

// stringToUTF16WithOffsets is the decoder above, optionally also recording where
// each code unit begins in the byte string. The offsets exist so that the
// UTF-16 <-> byte coordinate conversions can be O(1) after one classification
// instead of re-walking the string on every call.
//
// A surrogate pair records the START of its character for the lead unit and its
// END for the trail unit. That is deliberate: an offset landing between the
// halves of a pair has no byte position of its own, and rounding it up keeps
// every conversion a valid string boundary rather than a broken sequence.
//
// Both halves of a surrogate pair record the START of their character, so the
// offsets are non-decreasing and a byte offset can be mapped back by searching
// them. The round-up that a trail surrogate needs — it has no byte position of
// its own — is applied in UTF16ToByteOffset, where the tie identifies it.
//
// The returned offsets have n+1 entries, the last being len(s), so the end of
// the string is addressable without a special case.
func stringToUTF16WithOffsets(s string, wantOffsets bool) ([]uint16, []int32) {
	result := make([]uint16, 0, len(s))
	var offs []int32
	if wantOffsets {
		offs = make([]int32, 0, len(s)+1)
	}
	// one records a single code unit that begins at byte i.
	one := func(i int) {
		if wantOffsets {
			offs = append(offs, int32(i))
		}
	}
	bytes := []byte(s)
	i := 0

	for i < len(bytes) {
		b := bytes[i]
		if b < 0x80 {
			// ASCII
			result = append(result, uint16(b))
			one(i)
			i++
		} else if b < 0xC0 {
			// Invalid leading byte, treat as single byte
			result = append(result, uint16(b))
			one(i)
			i++
		} else if b < 0xE0 {
			// 2-byte sequence
			if i+1 < len(bytes) {
				r := rune(b&0x1F)<<6 | rune(bytes[i+1]&0x3F)
				result = append(result, uint16(r))
				one(i)
				i += 2
			} else {
				result = append(result, uint16(b))
				one(i)
				i++
			}
		} else if b < 0xF0 {
			// 3-byte sequence - check for WTF-8 surrogate encoding
			if i+2 < len(bytes) {
				b2 := bytes[i+1]
				b3 := bytes[i+2]
				// Decode the code point
				r := rune(b&0x0F)<<12 | rune(b2&0x3F)<<6 | rune(b3&0x3F)
				// This handles both regular BMP chars and WTF-8 surrogates
				result = append(result, uint16(r))
				one(i)
				i += 3
			} else {
				result = append(result, uint16(b))
				one(i)
				i++
			}
		} else if b < 0xF8 {
			// 4-byte sequence - supplementary character
			if i+3 < len(bytes) {
				r := rune(b&0x07)<<18 | rune(bytes[i+1]&0x3F)<<12 |
					rune(bytes[i+2]&0x3F)<<6 | rune(bytes[i+3]&0x3F)
				// Convert to surrogate pair
				r -= 0x10000
				high := uint16(0xD800 + (r >> 10))
				low := uint16(0xDC00 + (r & 0x3FF))
				result = append(result, high, low)
				if wantOffsets {
					// BOTH halves record the start of their character. Encoding
					// the trail as the character's END instead would collide
					// with the next character's start, and the reverse mapping
					// is a search over these offsets — it would land a unit
					// early. The round-up a trail surrogate needs is applied in
					// UTF16ToByteOffset, where a tie identifies it.
					offs = append(offs, int32(i), int32(i))
				}
				i += 4
			} else {
				result = append(result, uint16(b))
				one(i)
				i++
			}
		} else {
			// Invalid UTF-8 leading byte
			result = append(result, uint16(b))
			one(i)
			i++
		}
	}

	if wantOffsets {
		offs = append(offs, int32(len(bytes)))
	}
	return result, offs
}

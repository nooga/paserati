package vm

import (
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
		units := stringToUTF16Uncached(s)
		e = &utf16Entry{ptr: ptr, byteLen: len(s), n: len(units), ascii: false, units: units}
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
	result := make([]uint16, 0, len(s))
	bytes := []byte(s)
	i := 0

	for i < len(bytes) {
		b := bytes[i]
		if b < 0x80 {
			// ASCII
			result = append(result, uint16(b))
			i++
		} else if b < 0xC0 {
			// Invalid leading byte, treat as single byte
			result = append(result, uint16(b))
			i++
		} else if b < 0xE0 {
			// 2-byte sequence
			if i+1 < len(bytes) {
				r := rune(b&0x1F)<<6 | rune(bytes[i+1]&0x3F)
				result = append(result, uint16(r))
				i += 2
			} else {
				result = append(result, uint16(b))
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
				i += 3
			} else {
				result = append(result, uint16(b))
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
				i += 4
			} else {
				result = append(result, uint16(b))
				i++
			}
		} else {
			// Invalid UTF-8 leading byte
			result = append(result, uint16(b))
			i++
		}
	}

	return result
}

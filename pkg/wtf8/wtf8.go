// Package wtf8 holds the byte-level helpers that keep Paserati's WTF-8
// strings canonical.
//
// The VM stores JavaScript strings as Go strings in WTF-8: well-formed UTF-8,
// except that a lone UTF-16 surrogate (which UTF-8 cannot express) is kept as
// its generalized 3-byte form ED A0..BF xx. That gives one representation for
// every JS string - as long as two lone surrogates that together form a valid
// pair never sit next to each other, because then the same JS string (one
// supplementary character, two UTF-16 code units) would have two byte forms:
// the 4-byte UTF-8 sequence or the 6-byte surrogate pair. Equality and the
// UTF-16 view hide the difference, but every builtin that slices or scans Go
// bytes (TextEncoder, encodeURIComponent, regexps, spread, Intl.Segmenter,
// ...) would observe it.
//
// The WTF-8 spec therefore requires producers to re-pair: whenever a string
// is built from pieces (escape sequences in the lexer, concatenation, code
// unit sequences from fromCharCode/JSON.parse), an adjacent lead+trail pair
// must be encoded as the single code point it denotes. These helpers do that
// for the lexer and the VM, which cannot import each other.
package wtf8

import (
	"strings"
	"unicode/utf8"
)

// isLead reports whether s[i:i+3] is a WTF-8 lead surrogate (U+D800..U+DBFF).
func isLead(s string, i int) bool {
	return i+2 < len(s) && s[i] == 0xED && s[i+1] >= 0xA0 && s[i+1] <= 0xAF && s[i+2] >= 0x80 && s[i+2] <= 0xBF
}

// isTrail reports whether s[i:i+3] is a WTF-8 trail surrogate (U+DC00..U+DFFF).
func isTrail(s string, i int) bool {
	return i+2 < len(s) && s[i] == 0xED && s[i+1] >= 0xB0 && s[i+1] <= 0xBF && s[i+2] >= 0x80 && s[i+2] <= 0xBF
}

// codeUnit decodes the 3-byte generalized UTF-8 sequence at s[i:i+3].
func codeUnit(s string, i int) uint16 {
	return uint16(s[i]&0x0F)<<12 | uint16(s[i+1]&0x3F)<<6 | uint16(s[i+2]&0x3F)
}

func pairToRune(lead, trail uint16) rune {
	return 0x10000 + (rune(lead)-0xD800)<<10 + (rune(trail) - 0xDC00)
}

// JoinSurrogatePairs returns s with every adjacent lead+trail surrogate pair
// re-encoded as the 4-byte UTF-8 sequence of the supplementary character it
// denotes. Lone surrogates that do not pair stay as they are. When there is
// nothing to join, s itself is returned without allocating; strings with no
// 0xED byte (all ASCII, most text) take that path after one IndexByte.
func JoinSurrogatePairs(s string) string {
	i := strings.IndexByte(s, 0xED)
	if i < 0 {
		return s
	}
	var out []byte
	last := 0
	for i >= 0 && i+6 <= len(s) {
		if isLead(s, i) && isTrail(s, i+3) {
			if out == nil {
				out = make([]byte, 0, len(s))
			}
			out = append(out, s[last:i]...)
			out = utf8.AppendRune(out, pairToRune(codeUnit(s, i), codeUnit(s, i+3)))
			i += 6
			last = i
			if j := strings.IndexByte(s[i:], 0xED); j >= 0 {
				i += j
			} else {
				i = -1
			}
			continue
		}
		if j := strings.IndexByte(s[i+1:], 0xED); j >= 0 {
			i += 1 + j
		} else {
			i = -1
		}
	}
	if out == nil {
		return s
	}
	return string(append(out, s[last:]...))
}

// Concat returns a+b, re-pairing a lead surrogate at the end of a with a
// trail surrogate at the start of b. Both inputs are assumed canonical (as
// every VM string is), so only the seam needs checking - this is the
// constant-time helper for the hot concatenation paths.
func Concat(a, b string) string {
	if len(a) >= 3 && len(b) >= 3 && isLead(a, len(a)-3) && isTrail(b, 0) {
		var buf [4]byte
		n := utf8.EncodeRune(buf[:], pairToRune(codeUnit(a, len(a)-3), codeUnit(b, 0)))
		return a[:len(a)-3] + string(buf[:n]) + b[3:]
	}
	return a + b
}

// AppendCodeUnit appends one UTF-16 code unit to buf: BMP characters as
// UTF-8, surrogates in their 3-byte WTF-8 form. A sequence built this way
// must be passed through JoinSurrogatePairs once complete (or, when the
// units are all at hand, encoded with the VM's UTF16ToString instead).
func AppendCodeUnit(buf []byte, u uint16) []byte {
	if u >= 0xD800 && u <= 0xDFFF {
		return append(buf, 0xE0|byte(u>>12), 0x80|byte((u>>6)&0x3F), 0x80|byte(u&0x3F))
	}
	return utf8.AppendRune(buf, rune(u))
}

// WriteCodeUnit is AppendCodeUnit for a strings.Builder.
func WriteCodeUnit(sb *strings.Builder, u uint16) {
	var buf [3]byte
	sb.Write(AppendCodeUnit(buf[:0], u))
}

// HasSurrogate reports whether s contains a WTF-8 encoded surrogate (a lone
// one, in a canonical string). Cheap: one IndexByte for the common case.
func HasSurrogate(s string) bool {
	for i := strings.IndexByte(s, 0xED); i >= 0; {
		if i+1 < len(s) && s[i+1] >= 0xA0 {
			return true
		}
		j := strings.IndexByte(s[i+1:], 0xED)
		if j < 0 {
			return false
		}
		i += 1 + j
	}
	return false
}

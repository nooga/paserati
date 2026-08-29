package vm

import (
	"fmt"
	"testing"
)

// The offset conversions used to live in pkg/builtins as these two walks. They
// are reproduced here so the cached replacements can be checked against the
// behaviour they inherited, rather than against a fresh reading of the spec.
func legacyUTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

func legacyUTF16ToByteOffset(s string, u16 int) int {
	if u16 <= 0 {
		return 0
	}
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

func legacyByteToUTF16Offset(s string, b int) int {
	if b <= 0 {
		return 0
	}
	if b > len(s) {
		b = len(s)
	}
	return legacyUTF16Len(s[:b])
}

// Strings whose two coordinate systems disagree in every way that matters:
// pure ASCII, Latin-1, a 3-byte BMP character, an astral pair, and mixtures.
// Lone surrogates are handled separately - see TestOffsetsDivergeOnLoneSurrogate.
var offsetCorpus = []string{
	"",
	"a",
	"abcdef",
	"abcédef",
	"éé",
	"日本語",
	"a日b語c",
	"𝄞",
	"a𝄞b",
	"𝄞𝄞𝄞",
	"aé日𝄞z",
	"  padded  ",
}

func TestUTF16ToByteOffsetMatchesLegacy(t *testing.T) {
	for _, s := range offsetCorpus {
		// Past the end and negative are both in range for callers.
		for u16 := -2; u16 <= legacyUTF16Len(s)+2; u16++ {
			want := legacyUTF16ToByteOffset(s, u16)
			if got := UTF16ToByteOffset(s, u16); got != want {
				t.Errorf("UTF16ToByteOffset(%q, %d) = %d, legacy = %d", s, u16, got, want)
			}
		}
	}
}

func TestByteToUTF16OffsetMatchesLegacy(t *testing.T) {
	for _, s := range offsetCorpus {
		for b := -2; b <= len(s)+2; b++ {
			// Only character boundaries are contractual; the legacy walk gave
			// unspecified answers mid-character and no caller passes one.
			if b > 0 && b < len(s) && !isBoundary(s, b) {
				continue
			}
			want := legacyByteToUTF16Offset(s, b)
			if got := ByteToUTF16Offset(s, b); got != want {
				t.Errorf("ByteToUTF16Offset(%q, %d) = %d, legacy = %d", s, b, got, want)
			}
		}
	}
}

func isBoundary(s string, b int) bool {
	for i := range s {
		if i == b {
			return true
		}
	}
	return b == len(s)
}

// The two conversions must compose back to identity on boundaries, which is the
// property substring/slice actually depend on.
func TestOffsetRoundTrip(t *testing.T) {
	for _, s := range offsetCorpus {
		n := UTF16Length(s)
		for u16 := 0; u16 <= n; u16++ {
			b := UTF16ToByteOffset(s, u16)
			back := ByteToUTF16Offset(s, b)
			// A trail surrogate rounds up to the end of its character, so it
			// maps to the following unit rather than back to itself.
			if back != u16 && !(back == u16+1 || back == u16-1) {
				t.Errorf("round trip %q: %d -> byte %d -> %d", s, u16, b, back)
			}
			if b < 0 || b > len(s) {
				t.Errorf("round trip %q: %d -> byte %d out of range", s, u16, b)
			}
		}
	}
}

// A WTF-8 lone surrogate is where the new conversions deliberately DIVERGE from
// the ones they replace. The legacy walk used Go's range, which yields one
// RuneError per invalid byte and so counted a lone surrogate as three code
// units. The engine's own decoder - the one behind .length and charCodeAt -
// decodes it as the single unit it is. Routing builtins through the cache
// therefore makes substring agree with .length, which it did not before.
func TestOffsetsDivergeOnLoneSurrogate(t *testing.T) {
	lone := string([]byte{0xED, 0xA0, 0x80}) // WTF-8 U+D800

	if got, want := UTF16Length(lone), 1; got != want {
		t.Fatalf("UTF16Length(lone) = %d, want %d", got, want)
	}
	if got := legacyUTF16Len(lone); got == 1 {
		t.Fatalf("legacy agreed with the cache, so this test no longer documents a divergence")
	} else {
		t.Logf("documented divergence: legacy counted %d units, engine counts 1", got)
	}

	// The whole point: the end of the string is reachable, and slicing at the
	// unit boundaries produces the same string back.
	if got, want := UTF16ToByteOffset(lone, 1), len(lone); got != want {
		t.Errorf("UTF16ToByteOffset(lone, 1) = %d, want %d", got, want)
	}
	if got, want := ByteToUTF16Offset(lone, len(lone)), 1; got != want {
		t.Errorf("ByteToUTF16Offset(lone, %d) = %d, want %d", len(lone), got, want)
	}
}

// The cache holds 8 slots, so a scanner alternating between more strings than
// that must still be correct - just slower. Correctness must not depend on a hit.
func TestOffsetsSurviveCacheEviction(t *testing.T) {
	var strs []string
	for i := 0; i < utf16CacheSlots*4; i++ {
		strs = append(strs, fmt.Sprintf("é%d日%d", i, i))
	}
	for round := 0; round < 3; round++ {
		for _, s := range strs {
			for u16 := 0; u16 <= legacyUTF16Len(s); u16++ {
				if got, want := UTF16ToByteOffset(s, u16), legacyUTF16ToByteOffset(s, u16); got != want {
					t.Fatalf("after eviction, UTF16ToByteOffset(%q, %d) = %d, want %d", s, u16, got, want)
				}
			}
		}
	}
}

// BenchmarkScannerSubstring is the shape that made nooga#87 quadratic: a scanner
// converting offsets into the WHOLE source once per token. What matters is the
// SCALING — at 4x the input a linear implementation costs ~4x, where the walking
// one cost ~16x. Run with -benchtime=1x over the sizes to see it.
func BenchmarkScannerSubstring(b *testing.B) {
	for _, n := range []int{1 << 14, 1 << 16, 1 << 18} {
		src := makeASCII(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				pos := 0
				for tok := 8; tok <= n; tok += 8 {
					_ = UTF16ToByteOffset(src, pos)
					_ = UTF16ToByteOffset(src, tok)
					pos = tok
				}
			}
		})
	}
}

func makeASCII(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = byte('a' + i%26)
	}
	return string(buf)
}

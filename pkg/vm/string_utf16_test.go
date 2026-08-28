package vm

import (
	"math/rand"
	"strings"
	"testing"
)

// naiveIsASCII is the obvious byte-at-a-time reference for IsASCII.
func naiveIsASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// edgeStrings exercises the truncation and surrogate branches of the WTF-8
// decoder that the counting/indexing helpers must agree with.
var edgeStrings = []string{
	"",
	"a",
	"ascii only, all bytes < 0x80",
	strings.Repeat("x", 4096),
	"\xED\xA0\x80",                          // lone high surrogate (WTF-8) -> 1 code unit
	"\xED\xA0\x80abc",                       // lone surrogate followed by ASCII
	"\xED\xB0\x80",                          // lone low surrogate
	"\xF0\x9D\x90\x80",                      // U+1D400, astral -> surrogate pair (2 units)
	"a\xF0\x9D\x90\x80b",                    // astral between ASCII
	"café éè",                               // 2-byte sequences
	"\xC3",                                  // truncated 2-byte lead at end
	"\xE2\x82",                              // truncated 3-byte lead at end
	"\xF0\x9D\x90",                          // truncated 4-byte lead at end
	"\xF8\x80\x80\x80\x80",                  // 0xF8+ invalid lead
	"\x80\x80",                              // stray continuation bytes
	"世界",                                    // CJK, 3-byte BMP
	"mix \xE2\x82\xAC \xF0\x9F\x98\x80 end", // euro sign + emoji
}

func TestIsASCII(t *testing.T) {
	cases := append([]string{}, edgeStrings...)
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		n := r.Intn(40)
		b := make([]byte, n)
		for j := range b {
			b[j] = byte(r.Intn(256))
		}
		cases = append(cases, string(b))
	}
	for _, s := range cases {
		if got, want := IsASCII(s), naiveIsASCII(s); got != want {
			t.Fatalf("IsASCII(%q) = %v, want %v", s, got, want)
		}
	}
}

// TestUTF16HelpersMatchMaterialised checks that the cached length / index
// helpers agree, at every position, with a fresh full materialisation of the
// same string. The counting scan for non-ASCII strings is derived from the
// materialised slice by construction, so this mainly guards the ASCII fast path
// and the surrogate-pair combination in UTF16CodePointAt.
func TestUTF16HelpersMatchMaterialised(t *testing.T) {
	cases := append([]string{}, edgeStrings...)
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 5000; i++ {
		n := r.Intn(64)
		b := make([]byte, n)
		for j := range b {
			// Bias towards ASCII so the fast path gets real coverage, but
			// leave plenty of high bytes for the decoder branches.
			if r.Intn(3) == 0 {
				b[j] = byte(0x80 + r.Intn(128))
			} else {
				b[j] = byte(r.Intn(0x80))
			}
		}
		cases = append(cases, string(b))
	}

	for _, s := range cases {
		want := stringToUTF16Uncached(s)

		if got := UTF16Length(s); got != len(want) {
			t.Fatalf("UTF16Length(%q) = %d, want %d", s, got, len(want))
		}

		// Out-of-range probes.
		if _, ok := UTF16CodeUnitAt(s, -1); ok {
			t.Fatalf("UTF16CodeUnitAt(%q, -1) reported in range", s)
		}
		if _, ok := UTF16CodeUnitAt(s, len(want)); ok {
			t.Fatalf("UTF16CodeUnitAt(%q, len) reported in range", s)
		}

		for i := 0; i < len(want); i++ {
			u, ok := UTF16CodeUnitAt(s, i)
			if !ok || u != want[i] {
				t.Fatalf("UTF16CodeUnitAt(%q, %d) = %#x,%v want %#x,true", s, i, u, ok, want[i])
			}

			// UTF16CodePointAt: combine a trailing surrogate when present.
			cp, ok := UTF16CodePointAt(s, i)
			if !ok {
				t.Fatalf("UTF16CodePointAt(%q, %d) out of range", s, i)
			}
			wantCP := uint32(want[i])
			if want[i] >= 0xD800 && want[i] <= 0xDBFF && i+1 < len(want) &&
				want[i+1] >= 0xDC00 && want[i+1] <= 0xDFFF {
				wantCP = (uint32(want[i])-0xD800)*0x400 + (uint32(want[i+1]) - 0xDC00) + 0x10000
			}
			if cp != wantCP {
				t.Fatalf("UTF16CodePointAt(%q, %d) = %#x, want %#x", s, i, cp, wantCP)
			}
		}
	}
}

// TestUTF16CharAtPreservesLoneSurrogate makes sure charAt on a lone surrogate
// round-trips through WTF-8 rather than collapsing to U+FFFD.
func TestUTF16CharAtPreservesLoneSurrogate(t *testing.T) {
	s := "\xED\xA0\x80abc" // D800, a, b, c
	ch, ok := UTF16CharAt(s, 0)
	if !ok {
		t.Fatal("UTF16CharAt index 0 out of range")
	}
	if ch != "\xED\xA0\x80" {
		t.Fatalf("UTF16CharAt(loneSurrogate, 0) = %q, want WTF-8 D800", ch)
	}
	if got := StringToUTF16(ch); len(got) != 1 || got[0] != 0xD800 {
		t.Fatalf("round-trip of charAt result = %#v, want [D800]", got)
	}
	if ch, _ := UTF16CharAt(s, 1); ch != "a" {
		t.Fatalf("UTF16CharAt(_, 1) = %q, want \"a\"", ch)
	}
}

// TestUTF16CacheDistinctStringsSameSlot guards against a stale cache entry
// being served for a different string that hashes to the same slot.
func TestUTF16CacheDistinctStringsSameSlot(t *testing.T) {
	strs := make([]string, 512)
	for i := range strs {
		if i%2 == 0 {
			strs[i] = strings.Repeat("a", i%37+1)
		} else {
			strs[i] = strings.Repeat("é", i%37+1) // non-ASCII, 2 bytes each
		}
	}
	// Interleave so slots get overwritten between reads.
	for round := 0; round < 4; round++ {
		for _, s := range strs {
			want := len(stringToUTF16Uncached(s))
			if got := UTF16Length(s); got != want {
				t.Fatalf("UTF16Length(%q) = %d, want %d (round %d)", s, got, want, round)
			}
		}
	}
}

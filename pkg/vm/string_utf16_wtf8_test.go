package vm

import "testing"

// Half-character operations on a 4-byte UTF-8 character must yield WTF-8 lone
// surrogates, and needles holding lone surrogates must be found inside 4-byte
// characters - the cases a byte-level slice/search cannot express.
func TestUTF16SubstringSplitsSurrogatePairs(t *testing.T) {
	const grin = "😀" // U+1F600: D83D DE00
	lead, trail := "\xed\xa0\xbd", "\xed\xb8\x80"
	cases := []struct {
		s          string
		start, end int
		want       string
	}{
		{grin, 0, 2, grin},
		{grin, 0, 1, lead},
		{grin, 1, 2, trail},
		{grin, 1, 1, ""},
		{"a" + grin + "b", 1, 3, grin},
		{"a" + grin + "b", 2, 4, trail + "b"},
		{"a" + grin + "b", 0, 2, "a" + lead},
		{"plain", 1, 3, "la"},
		{"ünïcode", 1, 3, "nï"},
	}
	for _, c := range cases {
		if got := UTF16Substring(c.s, c.start, c.end); got != c.want {
			t.Errorf("UTF16Substring(%q, %d, %d) = %q, want %q", c.s, c.start, c.end, got, c.want)
		}
	}
	// Re-joining the halves reproduces the character's UTF-16 view.
	if UTF16ToString(StringToUTF16(UTF16Substring(grin, 0, 1)+UTF16Substring(grin, 1, 2))) != grin {
		t.Errorf("halves do not recombine")
	}
}

func TestUTF16SearchWithLoneSurrogates(t *testing.T) {
	const s = "a😀b😀"
	lead, trail := "\xed\xa0\xbd", "\xed\xb8\x80"
	if got := UTF16IndexOf(s, trail, 0); got != 2 {
		t.Errorf("IndexOf trail = %d, want 2", got)
	}
	if got := UTF16IndexOf(s, lead, 2); got != 4 {
		t.Errorf("IndexOf lead from 2 = %d, want 4", got)
	}
	if got := UTF16IndexOf(s, "😀b", 0); got != 1 {
		t.Errorf("IndexOf pair = %d, want 1", got)
	}
	if got := UTF16IndexOf(s, trail+"x", 0); got != -1 {
		t.Errorf("IndexOf missing = %d, want -1", got)
	}
	if got := UTF16LastIndexOf(s, lead, 10); got != 4 {
		t.Errorf("LastIndexOf lead = %d, want 4", got)
	}
	if got := UTF16LastIndexOf(s, lead, 3); got != 1 {
		t.Errorf("LastIndexOf lead from 3 = %d, want 1", got)
	}
	if !UTF16HasPrefixAt(s, lead, 1) || UTF16HasPrefixAt(s, lead, 2) {
		t.Errorf("HasPrefixAt wrong")
	}
	if !UTF16HasSuffixAt(s, trail, 6) || UTF16HasSuffixAt(s, trail, 5) {
		t.Errorf("HasSuffixAt wrong")
	}
}

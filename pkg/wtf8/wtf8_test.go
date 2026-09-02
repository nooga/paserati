package wtf8

import (
	"strings"
	"testing"
)

const (
	lead  = "\xed\xa0\x80" // U+D800
	trail = "\xed\xb0\x80" // U+DC00
	pair  = "\U00010000"   // what lead+trail denote
	// U+D7FF is the last code point before the surrogate block: ED 9F BF,
	// the closest well-formed neighbour of the lead pattern.
	preSurrogate = "퟿"
)

func TestJoinSurrogatePairs(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		"plain ascii":               "plain ascii",
		lead + trail:                pair,
		"a" + lead + trail + "b":    "a" + pair + "b",
		lead:                        lead,
		trail:                       trail,
		trail + lead:                trail + lead,
		lead + lead + trail:         lead + pair,
		lead + trail + lead + trail: pair + pair,
		preSurrogate + trail:        preSurrogate + trail,
		"한글" + lead + trail:         "한글" + pair, // Hangul also uses 0xED lead bytes
		pair:                        pair,
		lead + "x" + trail:          lead + "x" + trail,
	}
	for in, want := range cases {
		if got := JoinSurrogatePairs(in); got != want {
			t.Errorf("JoinSurrogatePairs(%q) = %q, want %q", in, got, want)
		}
	}
	// No allocation / identical string when nothing changes.
	s := "already canonical " + pair + lead
	if got := JoinSurrogatePairs(s); got != s {
		t.Errorf("canonical input changed: %q", got)
	}
}

func TestConcat(t *testing.T) {
	if got := Concat(lead, trail); got != pair {
		t.Errorf("Concat(lead, trail) = %q", got)
	}
	if got := Concat("ab"+lead, trail+"cd"); got != "ab"+pair+"cd" {
		t.Errorf("Concat with context = %q", got)
	}
	for _, c := range [][2]string{{"a", "b"}, {lead, "x"}, {"x", trail}, {trail, lead}, {"", trail}, {lead, ""}, {preSurrogate, trail}} {
		if got := Concat(c[0], c[1]); got != c[0]+c[1] {
			t.Errorf("Concat(%q, %q) = %q, want plain concatenation", c[0], c[1], got)
		}
	}
}

func TestAppendCodeUnit(t *testing.T) {
	var sb strings.Builder
	for _, u := range []uint16{'a', 0xD800, 0xDC00, 0xD7FF, 0xFFFD} {
		WriteCodeUnit(&sb, u)
	}
	want := "a" + lead + trail + preSurrogate + "�"
	if sb.String() != want {
		t.Errorf("WriteCodeUnit sequence = %q, want %q", sb.String(), want)
	}
	if got := JoinSurrogatePairs(sb.String()); got != "a"+pair+preSurrogate+"�" {
		t.Errorf("joined sequence = %q", got)
	}
}

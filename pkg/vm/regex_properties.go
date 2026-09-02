package vm

import (
	"strings"
	"sync"
	"unicode"
)

// ECMAScript's Unicode property escapes include two derived binary
// properties - ID_Start and ID_Continue (UAX #31, the definitions JavaScript
// itself uses for identifier characters) - that neither RE2 nor regexp2
// recognizes by name (paserati#190). Both engines do accept explicit
// character-class ranges, and both derived properties are fixed set algebra
// over categories Go's unicode package already ships, so the tables are
// computed here once and spliced into the pattern as ranges before either
// engine parses it.
//
// The expansion is unconditional on flags, matching how the two engines
// already treat every other \p{...} name in this runtime.
//
// Coverage follows Go's own Unicode version (unicode.Version), so code points
// newer than that are absent from both sets until the toolchain catches up.

// runeRange is an inclusive [lo, hi] span of code points.
type runeRange struct{ lo, hi rune }

// derivedProperty holds a property's member spans and their gaps.
type derivedProperty struct {
	ranges     []runeRange
	complement []runeRange
}

var (
	derivedOnce        sync.Once
	idStartProperty    derivedProperty
	idContinueProperty derivedProperty
)

// derivedUnicodeProperties maps every spelling ECMAScript accepts for the two
// properties (the canonical names and their UCD aliases) to their tables.
// Property names are case-sensitive in ECMAScript, so the lookup is too.
var derivedUnicodeProperties = map[string]*derivedProperty{
	"ID_Start":    &idStartProperty,
	"IDS":         &idStartProperty,
	"ID_Continue": &idContinueProperty,
	"IDC":         &idContinueProperty,
}

// buildDerivedProperties fills both tables on first use, in a few
// milliseconds: membership is painted into a bitmap straight from the source
// tables' ranges rather than probed rune by rune across the code space.
//
//	ID_Start    = L + Nl + Other_ID_Start - Pattern_Syntax - Pattern_White_Space
//	ID_Continue = ID_Start + Mn + Mc + Nd + Pc + Other_ID_Continue
//	              - Pattern_Syntax - Pattern_White_Space
func buildDerivedProperties() {
	derivedOnce.Do(func() {
		set := newRuneSet()
		set.include(unicode.L, unicode.Nl, unicode.Other_ID_Start)
		set.exclude(unicode.Pattern_Syntax, unicode.Pattern_White_Space)
		idStartProperty = set.property()

		set.include(unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Other_ID_Continue)
		set.exclude(unicode.Pattern_Syntax, unicode.Pattern_White_Space)
		idContinueProperty = set.property()
	})
}

// runeSet is one bit per code point.
type runeSet []uint64

func newRuneSet() runeSet { return make(runeSet, (unicode.MaxRune+1+63)/64) }

func (s runeSet) has(r rune) bool { return s[r>>6]&(1<<(uint(r)&63)) != 0 }

func (s runeSet) paint(tables []*unicode.RangeTable, on bool) {
	for _, t := range tables {
		for _, r := range t.R16 {
			for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
				s.set(c, on)
			}
		}
		for _, r := range t.R32 {
			for c := rune(r.Lo); c <= rune(r.Hi); c += rune(r.Stride) {
				s.set(c, on)
			}
		}
	}
}

func (s runeSet) set(r rune, on bool) {
	if on {
		s[r>>6] |= 1 << (uint(r) & 63)
	} else {
		s[r>>6] &^= 1 << (uint(r) & 63)
	}
}

func (s runeSet) include(tables ...*unicode.RangeTable) { s.paint(tables, true) }
func (s runeSet) exclude(tables ...*unicode.RangeTable) { s.paint(tables, false) }

// property reads the bitmap back out as member spans and their gaps.
// Surrogates are left out of the complement: a Go string can't carry one as
// a literal rune, and no UTF-8 subject can contain one either.
func (s runeSet) property() derivedProperty {
	var p derivedProperty
	inRun := false
	var start rune
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if !inRun && s[r>>6] == 0 {
			// Skip an empty word of the bitmap in one step.
			r |= 63
			continue
		}
		if s.has(r) {
			if !inRun {
				start, inRun = r, true
			}
			continue
		}
		if inRun {
			p.ranges = append(p.ranges, runeRange{start, r - 1})
			inRun = false
		}
	}
	if inRun {
		p.ranges = append(p.ranges, runeRange{start, unicode.MaxRune})
	}

	next := rune(0)
	for _, rr := range p.ranges {
		if rr.lo > next {
			p.complement = appendWithoutSurrogates(p.complement, next, rr.lo-1)
		}
		next = rr.hi + 1
	}
	if next <= unicode.MaxRune {
		p.complement = appendWithoutSurrogates(p.complement, next, unicode.MaxRune)
	}
	return p
}

func appendWithoutSurrogates(dst []runeRange, lo, hi rune) []runeRange {
	const surrogateLo, surrogateHi = 0xD800, 0xDFFF
	if hi < surrogateLo || lo > surrogateHi {
		return append(dst, runeRange{lo, hi})
	}
	if lo < surrogateLo {
		dst = append(dst, runeRange{lo, surrogateLo - 1})
	}
	if hi > surrogateHi {
		dst = append(dst, runeRange{surrogateHi + 1, hi})
	}
	return dst
}

// writeClassRune spells one code point as a character-class member both
// engines read the same way: \xHH for ASCII (which also keeps the class
// metacharacters and control characters out of the pattern text), the
// literal rune for everything else. Neither engine shares an escape for
// astral code points (RE2 wants \x{...}, regexp2 knows only \uHHHH), but
// both are rune-based and take the character itself.
func writeClassRune(b *strings.Builder, r rune) {
	if r < 0x80 {
		const hex = "0123456789abcdef"
		b.WriteString(`\x`)
		b.WriteByte(hex[r>>4])
		b.WriteByte(hex[r&0xf])
		return
	}
	b.WriteRune(r)
}

func writeClassRanges(b *strings.Builder, ranges []runeRange) {
	for _, rr := range ranges {
		writeClassRune(b, rr.lo)
		if rr.hi != rr.lo {
			b.WriteByte('-')
			writeClassRune(b, rr.hi)
		}
	}
}

// expandDerivedUnicodeProperties rewrites \p{ID_Start}, \p{ID_Continue} and
// their \P negations (plus the IDS/IDC aliases) into explicit ranges. Inside
// a character class only the members are emitted; outside, they're wrapped
// in a class of their own. Every other escape - including every other
// \p{...} name - passes through untouched for the engines to judge.
//
// Operates on bytes like rewriteECMAClasses: every construct inspected is
// ASCII, so multi-byte runes copy through as-is.
func expandDerivedUnicodeProperties(pattern string) string {
	if !strings.Contains(pattern, `\p{`) && !strings.Contains(pattern, `\P{`) {
		return pattern
	}
	var b strings.Builder
	b.Grow(len(pattern))
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			if e := pattern[i+1]; (e == 'p' || e == 'P') && i+2 < len(pattern) && pattern[i+2] == '{' {
				if end := strings.IndexByte(pattern[i+3:], '}'); end >= 0 {
					if prop, ok := derivedUnicodeProperties[pattern[i+3:i+3+end]]; ok {
						buildDerivedProperties()
						ranges := prop.ranges
						if e == 'P' {
							ranges = prop.complement
						}
						if !inClass {
							b.WriteByte('[')
						}
						writeClassRanges(&b, ranges)
						if !inClass {
							b.WriteByte(']')
						}
						i += 3 + end
						continue
					}
				}
			}
			// Any other escape passes through with its operand, which also
			// keeps \\ and \[ from being read as syntax below.
			b.WriteByte(c)
			b.WriteByte(pattern[i+1])
			i++
			continue
		}
		switch {
		case c == '[' && !inClass:
			inClass = true
		case c == ']' && inClass:
			inClass = false
		}
		b.WriteByte(c)
	}
	return b.String()
}

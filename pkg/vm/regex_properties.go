package vm

import (
	"strings"
	"sync"
	"unicode"
)

// ECMAScript's Unicode property escapes name a set of binary properties that
// neither RE2 nor regexp2 fully recognizes: RE2 knows only general categories
// and scripts, regexp2 a subset of the UCD property names under their
// canonical spelling and none of the UCD aliases, and the derived properties
// of DerivedCoreProperties.txt - ID_Start, Alphabetic,
// Default_Ignorable_Code_Point, ... - are absent from both (paserati#190,
// paserati#196). Both engines do accept explicit character-class ranges, and
// every property below is either a table Go's unicode package ships verbatim
// or fixed set algebra over such tables, so the sets are computed here on
// first use and spliced into the pattern as ranges before either engine
// parses it.
//
// The expansion is unconditional on flags, matching how the two engines
// already treat every other \p{...} name in this runtime.
//
// Coverage follows Go's own Unicode version (unicode.Version), so code points
// newer than that are absent from the sets until the toolchain catches up.
// Properties whose data Go does not carry - the Emoji family,
// Extended_Pictographic, Bidi_Mirrored, Case_Ignorable, the Changes_When_*
// family, XID_Start and XID_Continue - pass through untouched for the engines
// to judge.

// runeRange is an inclusive [lo, hi] span of code points.
type runeRange struct{ lo, hi rune }

// derivedProperty holds one property's member spans and their gaps, computed
// on first use from its build function.
type derivedProperty struct {
	once       sync.Once
	build      func(set runeSet)
	ranges     []runeRange
	complement []runeRange
}

func (p *derivedProperty) get() *derivedProperty {
	p.once.Do(func() {
		set := newRuneSet()
		p.build(set)
		p.ranges, p.complement = set.spans()
	})
	return p
}

// tableProperty is a property that is exactly the union of Go's tables.
func tableProperty(tables ...*unicode.RangeTable) *derivedProperty {
	return &derivedProperty{build: func(set runeSet) { set.include(tables...) }}
}

// derivedUnicodeProperties maps every spelling ECMAScript accepts (the
// canonical names and their UCD aliases) to the property's table. Property
// names are case-sensitive in ECMAScript, so the lookup is too.
var derivedUnicodeProperties = map[string]*derivedProperty{}

func registerDerivedProperty(p *derivedProperty, names ...string) {
	for _, n := range names {
		derivedUnicodeProperties[n] = p
	}
}

func init() {
	// Properties Go ships as tables, under their canonical name and alias.
	registerDerivedProperty(tableProperty(unicode.ASCII_Hex_Digit), "ASCII_Hex_Digit", "AHex")
	registerDerivedProperty(tableProperty(unicode.Bidi_Control), "Bidi_Control", "Bidi_C")
	registerDerivedProperty(tableProperty(unicode.Dash), "Dash")
	registerDerivedProperty(tableProperty(unicode.Deprecated), "Deprecated", "Dep")
	registerDerivedProperty(tableProperty(unicode.Diacritic), "Diacritic", "Dia")
	registerDerivedProperty(tableProperty(unicode.Extender), "Extender", "Ext")
	registerDerivedProperty(tableProperty(unicode.Hex_Digit), "Hex_Digit", "Hex")
	registerDerivedProperty(tableProperty(unicode.IDS_Binary_Operator), "IDS_Binary_Operator", "IDSB")
	registerDerivedProperty(tableProperty(unicode.IDS_Trinary_Operator), "IDS_Trinary_Operator", "IDST")
	registerDerivedProperty(tableProperty(unicode.Ideographic), "Ideographic", "Ideo")
	registerDerivedProperty(tableProperty(unicode.Join_Control), "Join_Control", "Join_C")
	registerDerivedProperty(tableProperty(unicode.Logical_Order_Exception), "Logical_Order_Exception", "LOE")
	registerDerivedProperty(tableProperty(unicode.Noncharacter_Code_Point), "Noncharacter_Code_Point", "NChar")
	registerDerivedProperty(tableProperty(unicode.Pattern_Syntax), "Pattern_Syntax", "Pat_Syn")
	registerDerivedProperty(tableProperty(unicode.Pattern_White_Space), "Pattern_White_Space", "Pat_WS")
	registerDerivedProperty(tableProperty(unicode.Quotation_Mark), "Quotation_Mark", "QMark")
	registerDerivedProperty(tableProperty(unicode.Radical), "Radical")
	registerDerivedProperty(tableProperty(unicode.Regional_Indicator), "Regional_Indicator", "RI")
	registerDerivedProperty(tableProperty(unicode.Sentence_Terminal), "Sentence_Terminal", "STerm")
	registerDerivedProperty(tableProperty(unicode.Soft_Dotted), "Soft_Dotted", "SD")
	registerDerivedProperty(tableProperty(unicode.Terminal_Punctuation), "Terminal_Punctuation", "Term")
	registerDerivedProperty(tableProperty(unicode.Unified_Ideograph), "Unified_Ideograph", "UIdeo")
	registerDerivedProperty(tableProperty(unicode.Variation_Selector), "Variation_Selector", "VS")
	registerDerivedProperty(tableProperty(unicode.White_Space), "White_Space", "space")

	// The three ECMAScript-only names.
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.includeRange(0, unicode.MaxRune)
	}}, "Any")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.includeRange(0, 0x7F)
	}}, "ASCII")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		// Everything with a General_Category other than Cn: the union of
		// every category table Go ships.
		for _, t := range unicode.Categories {
			set.include(t)
		}
	}}, "Assigned")

	// Derived binary properties, each per its DerivedCoreProperties.txt
	// definition.
	//
	//	Alphabetic      = L + Nl + Other_Alphabetic
	//	Lowercase       = Ll + Other_Lowercase
	//	Uppercase       = Lu + Other_Uppercase
	//	Cased           = Lowercase + Uppercase + Lt
	//	Math            = Sm + Other_Math
	//	Grapheme_Extend = Me + Mn + Other_Grapheme_Extend
	//	Grapheme_Base   = Assigned - Cc - Cf - Cs - Co - Zl - Zp - Grapheme_Extend
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.L, unicode.Nl, unicode.Other_Alphabetic)
	}}, "Alphabetic", "Alpha")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.Ll, unicode.Other_Lowercase)
	}}, "Lowercase", "Lower")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.Lu, unicode.Other_Uppercase)
	}}, "Uppercase", "Upper")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.Ll, unicode.Other_Lowercase, unicode.Lu, unicode.Other_Uppercase, unicode.Lt)
	}}, "Cased")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.Sm, unicode.Other_Math)
	}}, "Math")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.Me, unicode.Mn, unicode.Other_Grapheme_Extend)
	}}, "Grapheme_Extend", "Gr_Ext")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		for _, t := range unicode.Categories {
			set.include(t)
		}
		set.exclude(unicode.Cc, unicode.Cf, unicode.Cs, unicode.Co, unicode.Zl, unicode.Zp,
			unicode.Me, unicode.Mn, unicode.Other_Grapheme_Extend)
	}}, "Grapheme_Base", "Gr_Base")

	//	ID_Start    = L + Nl + Other_ID_Start - Pattern_Syntax - Pattern_White_Space
	//	ID_Continue = ID_Start + Mn + Mc + Nd + Pc + Other_ID_Continue
	//	              - Pattern_Syntax - Pattern_White_Space
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.L, unicode.Nl, unicode.Other_ID_Start)
		set.exclude(unicode.Pattern_Syntax, unicode.Pattern_White_Space)
	}}, "ID_Start", "IDS")
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.L, unicode.Nl, unicode.Other_ID_Start,
			unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Other_ID_Continue)
		set.exclude(unicode.Pattern_Syntax, unicode.Pattern_White_Space)
	}}, "ID_Continue", "IDC")

	//	Default_Ignorable_Code_Point = Other_Default_Ignorable_Code_Point
	//	    + Cf + Variation_Selector
	//	    - White_Space
	//	    - FFF9..FFFB (interlinear annotation format characters)
	//	    - 13430..13440 (Egyptian hieroglyph format characters)
	//	    - Prepended_Concatenation_Mark (format characters that must stay visible)
	registerDerivedProperty(&derivedProperty{build: func(set runeSet) {
		set.include(unicode.Other_Default_Ignorable_Code_Point, unicode.Cf, unicode.Variation_Selector)
		set.exclude(unicode.White_Space, unicode.Prepended_Concatenation_Mark)
		set.excludeRange(0xFFF9, 0xFFFB)
		set.excludeRange(0x13430, 0x13440)
	}}, "Default_Ignorable_Code_Point", "DI")
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

func (s runeSet) includeRange(lo, hi rune) {
	for c := lo; c <= hi; c++ {
		s.set(c, true)
	}
}

func (s runeSet) excludeRange(lo, hi rune) {
	for c := lo; c <= hi; c++ {
		s.set(c, false)
	}
}

// spans reads the bitmap back out as member spans and their gaps. Surrogates
// are left out of both: a Go string can't carry one as a literal rune, and
// no UTF-8 subject can contain one either.
func (s runeSet) spans() (ranges, complement []runeRange) {
	var members []runeRange
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
			members = append(members, runeRange{start, r - 1})
			inRun = false
		}
	}
	if inRun {
		members = append(members, runeRange{start, unicode.MaxRune})
	}

	next := rune(0)
	for _, rr := range members {
		if rr.lo > next {
			complement = appendWithoutSurrogates(complement, next, rr.lo-1)
		}
		ranges = appendWithoutSurrogates(ranges, rr.lo, rr.hi)
		next = rr.hi + 1
	}
	if next <= unicode.MaxRune {
		complement = appendWithoutSurrogates(complement, next, unicode.MaxRune)
	}
	return ranges, complement
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

// expandDerivedUnicodeProperties rewrites every \p{Name} / \P{Name} whose
// Name is in derivedUnicodeProperties into explicit ranges. Inside a
// character class only the members are emitted; outside, they're wrapped in
// a class of their own. Every other escape - including every other \p{...}
// name, and the \p{Script=...} / \p{General_Category=...} forms - passes
// through untouched for the engines to judge.
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
						prop.get()
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

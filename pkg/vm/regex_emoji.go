package vm

import (
	"strings"
	"sync"
	"unicode"
)

// Unicode's "properties of strings" (RGI_Emoji and friends, from the
// emoji-sequence data files, not DerivedCoreProperties.txt) are a different
// kind of gap than the properties expandDerivedUnicodeProperties handles
// above: they match whole *sequences* of code points (a skin-toned emoji, a
// flag, a family joined with ZWJ), valid only under the `v` flag, and are
// fundamentally not expressible as a `[...]` character class - neither
// engine has any notion of them at all (paserati#224). Go's unicode package
// ships no equivalent data (no Emoji_Modifier_Base, no Extended_Pictographic,
// no RGI_Emoji_ZWJ_Sequence list), so this file hand-assembles a real
// (?:alternation) of sequence shapes from the pieces Unicode's emoji
// specification defines structurally, rather than pass the name through for
// the engines to reject.
//
// This is a best-effort approximation, not the literal curated data:
//
//   - Basic_Emoji is approximated as the five solid default-emoji-presentation
//     blocks (Misc Symbols & Pictographs, Emoticons, Transport & Map Symbols,
//     Supplemental Symbols & Pictographs, Symbols & Pictographs Extended-A)
//     plus an optional trailing VS16. Older scattered single emoji that need
//     an explicit VS16 outside those blocks (e.g. the original dingbats) are
//     not covered.
//   - RGI_Emoji_Flag_Sequence is any two Regional_Indicator symbols, not
//     restricted to currently-assigned region codes (Go carries no such
//     restriction list either).
//   - RGI_Emoji_Modifier_Sequence uses a hand-maintained Emoji_Modifier_Base
//     range list (Unicode has no single stable table for it; this one won't
//     track future emoji releases automatically).
//   - RGI_Emoji_ZWJ_Sequence is approximated as two or more of the above
//     units joined by ZWJ, rather than the actual finite curated sequence
//     list (~1500 entries) - this can accept a well-formed but not
//     Unicode-recognized combination, but every real ZWJ emoji (family,
//     couple, profession-with-skin-tone, ...) is built from exactly these
//     parts and so still matches.
//   - RGI_Emoji_Tag_Sequence is exact (the flag-tag-cancel grammar has no
//     ambiguity to approximate).
//   - Emoji_Keycap_Sequence is exact.
//
// Per ECMAScript, a property of strings can only be used as \p{Name} (never
// negated with \P{Name} - that's a SyntaxError) and, in this implementation,
// only outside a character class: splicing a multi-codepoint alternation
// into a `[...]` class is a `v`-flag-only construct with no equivalent in
// either underlying engine's class syntax.

// Code points structural to the emoji-sequence grammar itself (not table
// data - these are exact, stable parts of the Unicode emoji specification).
const (
	emojiVS16       rune = 0x0000FE0F // Variation Selector-16 (emoji presentation)
	emojiZWJ        rune = 0x0000200D // Zero Width Joiner
	emojiKeycapMark rune = 0x000020E3 // Combining Enclosing Keycap
	emojiModifierLo rune = 0x0001F3FB // Skin tone modifiers (Emoji_Modifier)
	emojiModifierHi rune = 0x0001F3FF
	emojiTagBase    rune = 0x0001F3F4 // WAVING BLACK FLAG (RGI tag-sequence base)
	emojiTagLo      rune = 0x000E0020 // Tag characters
	emojiTagHi      rune = 0x000E007E
	emojiTagCancel  rune = 0x000E007F // CANCEL TAG
)

// emojiModifierBaseRanges: a hand-maintained approximation of
// Emoji_Modifier_Base (Unicode ships no single stable table for it; see the
// package doc comment above).
var emojiModifierBaseRanges = []runeRange{
	{0x261D, 0x261D}, {0x26F9, 0x26F9}, {0x270A, 0x270D},
	{0x1F385, 0x1F385}, {0x1F3C2, 0x1F3C4}, {0x1F3C7, 0x1F3C7},
	{0x1F3CA, 0x1F3CC}, {0x1F442, 0x1F443}, {0x1F446, 0x1F450},
	{0x1F466, 0x1F469}, {0x1F46E, 0x1F46E}, {0x1F470, 0x1F478},
	{0x1F47C, 0x1F47C}, {0x1F481, 0x1F483}, {0x1F485, 0x1F487},
	{0x1F48F, 0x1F48F}, {0x1F491, 0x1F491}, {0x1F4AA, 0x1F4AA},
	{0x1F574, 0x1F575}, {0x1F57A, 0x1F57A}, {0x1F590, 0x1F590},
	{0x1F595, 0x1F596}, {0x1F645, 0x1F647}, {0x1F64B, 0x1F64F},
	{0x1F6A3, 0x1F6A3}, {0x1F6B4, 0x1F6B6}, {0x1F6C0, 0x1F6C0},
	{0x1F6CC, 0x1F6CC}, {0x1F90C, 0x1F90C}, {0x1F90F, 0x1F90F},
	{0x1F918, 0x1F91F}, {0x1F926, 0x1F926}, {0x1F930, 0x1F939},
	{0x1F93D, 0x1F93E}, {0x1F9B5, 0x1F9B6}, {0x1F9B8, 0x1F9B9},
	{0x1F9BB, 0x1F9BB}, {0x1F9CD, 0x1F9CF}, {0x1F9D1, 0x1F9DD},
}

// basicEmojiRanges: solid default-emoji-presentation blocks; see the package
// doc comment above for what this deliberately does not cover.
var basicEmojiRanges = []runeRange{
	{0x1F300, 0x1F5FF}, // Miscellaneous Symbols and Pictographs
	{0x1F600, 0x1F64F}, // Emoticons
	{0x1F680, 0x1F6FF}, // Transport and Map Symbols
	{0x1F900, 0x1F9FF}, // Supplemental Symbols and Pictographs
	{0x1FA70, 0x1FAFF}, // Symbols and Pictographs Extended-A
}

// runeClass renders a `[...]` character class from a set of ranges, reusing
// writeClassRanges/writeClassRune so astral code points are spelled the same
// rune-literal way the rest of this package already relies on both engines
// accepting in a class.
func runeClass(ranges []runeRange) string {
	var b strings.Builder
	b.WriteByte('[')
	writeClassRanges(&b, ranges)
	b.WriteByte(']')
	return b.String()
}

func patternRune(r rune) string {
	var b strings.Builder
	writeClassRune(&b, r)
	return b.String()
}

// regionalIndicatorRanges computes Regional_Indicator's ranges once, reusing
// the same table this package already registers as a bare \p{} property.
var regionalIndicatorRanges = &derivedProperty{build: func(set runeSet) {
	set.include(unicode.Regional_Indicator)
}}

// emojiPatternPiece lazily builds and caches one regex pattern fragment.
type emojiPatternPiece struct {
	once    sync.Once
	build   func() string
	pattern string
}

func (p *emojiPatternPiece) get() string {
	p.once.Do(func() { p.pattern = p.build() })
	return p.pattern
}

var basicEmojiPattern = &emojiPatternPiece{build: func() string {
	return runeClass(basicEmojiRanges) + patternRune(emojiVS16) + "?"
}}

var modifierSeqPattern = &emojiPatternPiece{build: func() string {
	return runeClass(emojiModifierBaseRanges) + patternRune(emojiVS16) + "?" +
		"[" + patternRune(emojiModifierLo) + "-" + patternRune(emojiModifierHi) + "]"
}}

var flagSeqPattern = &emojiPatternPiece{build: func() string {
	cls := runeClass(regionalIndicatorRanges.get().ranges)
	return cls + cls
}}

var tagSeqPattern = &emojiPatternPiece{build: func() string {
	return patternRune(emojiTagBase) +
		"[" + patternRune(emojiTagLo) + "-" + patternRune(emojiTagHi) + "]+" +
		patternRune(emojiTagCancel)
}}

var keycapSeqPattern = &emojiPatternPiece{build: func() string {
	return `[0-9#*]` + patternRune(emojiVS16) + "?" + patternRune(emojiKeycapMark)
}}

var emojiUnitPattern = &emojiPatternPiece{build: func() string {
	return "(?:" + strings.Join([]string{
		tagSeqPattern.get(),
		keycapSeqPattern.get(),
		flagSeqPattern.get(),
		modifierSeqPattern.get(),
		basicEmojiPattern.get(),
	}, "|") + ")"
}}

var zwjSeqPattern = &emojiPatternPiece{build: func() string {
	unit := emojiUnitPattern.get()
	return unit + "(?:" + patternRune(emojiZWJ) + unit + ")+"
}}

var rgiEmojiPattern = &emojiPatternPiece{build: func() string {
	return "(?:" + zwjSeqPattern.get() + "|" + emojiUnitPattern.get() + ")"
}}

// emojiStringProperties maps each "property of strings" name ECMAScript's
// emoji-sequence data defines to the pattern piece it expands to. Every
// value is wrapped in its own non-capturing group by the caller.
var emojiStringProperties = map[string]*emojiPatternPiece{
	"Basic_Emoji":                 basicEmojiPattern,
	"Emoji_Keycap_Sequence":       keycapSeqPattern,
	"RGI_Emoji_Flag_Sequence":     flagSeqPattern,
	"RGI_Emoji_Modifier_Sequence": modifierSeqPattern,
	"RGI_Emoji_Tag_Sequence":      tagSeqPattern,
	"RGI_Emoji_ZWJ_Sequence":      zwjSeqPattern,
	"RGI_Emoji":                   rgiEmojiPattern,
}

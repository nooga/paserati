package vm

import (
	"strings"
	"sync"
)

// Unicode's "properties of strings" (Basic_Emoji, RGI_Emoji and friends,
// valid only under the v flag) match whole sequences of code points - a
// keycap, a flag, a skin-toned emoji - so they can't be character classes.
// Each is its list of members (regex_unicode_data.go), matched as an
// alternation of them, longest first, ahead of a class of the single code
// point members.
//
// RGI_Emoji_ZWJ_Sequence (and so RGI_Emoji, which includes it) is the one
// whose member list isn't carried: it is approximated by its grammar, two or
// more emoji elements joined by ZWJ. That accepts every real ZWJ sequence,
// but also well-formed combinations Unicode doesn't recommend, and it can't
// take part in class intersection or subtraction.

const (
	emojiVS16       rune = 0xFE0F  // Variation Selector-16 (emoji presentation)
	emojiZWJ        rune = 0x200D  // Zero Width Joiner
	emojiModifierLo rune = 0x1F3FB // Skin tone modifiers (Emoji_Modifier)
	emojiModifierHi rune = 0x1F3FF
)

// stringPropertyValue returns the members of a property of strings, or nil
// for a name that isn't one.
func stringPropertyValue(name string) *classValue {
	v := &classValue{chars: newCharSet()}
	addMembers := func(name string) {
		for _, s := range generatedStringProperties[name] {
			v.addString([]rune(s))
		}
	}
	switch name {
	case "RGI_Emoji_ZWJ_Sequence":
		v.pieces = []string{zwjSequencePattern()}
	case "RGI_Emoji":
		for n := range generatedStringProperties {
			addMembers(n)
		}
		v.pieces = []string{zwjSequencePattern()}
	default:
		if _, ok := generatedStringProperties[name]; !ok {
			return nil
		}
		addMembers(name)
	}
	return v
}

// zwjSequencePattern is the approximation of RGI_Emoji_ZWJ_Sequence:
// elements - an emoji with a skin tone modifier or an optional VS16 - joined
// by ZWJ.
var zwjSequencePattern = sync.OnceValue(func() string {
	var b strings.Builder
	b.WriteString("(?:")
	writeRangesClass(&b, lookupUnicodeProperty("Emoji_Modifier_Base").ranges)
	b.WriteByte('[')
	writeSetRune(&b, emojiModifierLo)
	b.WriteByte('-')
	writeSetRune(&b, emojiModifierHi)
	b.WriteString("]|")
	writeRangesClass(&b, lookupUnicodeProperty("Emoji").ranges)
	writeSetRune(&b, emojiVS16)
	b.WriteString("?)")
	element := b.String()
	return element + "(?:" + string(emojiZWJ) + element + ")+"
})

func writeRangesClass(b *strings.Builder, ranges []runeRange) {
	b.WriteByte('[')
	for _, rr := range ranges {
		writeSetRune(b, rr.lo)
		if rr.hi != rr.lo {
			b.WriteByte('-')
			writeSetRune(b, rr.hi)
		}
	}
	b.WriteByte(']')
}

package vm

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"sync"
	"unicode"

	"github.com/nooga/paserati/pkg/jsregex"
)

// ECMAScript's Unicode property escapes (\p{...} and \P{...} under the u and
// v flags) name General_Category values, Script and Script_Extensions values
// and 53 binary properties, each under several aliases. Neither RE2 nor
// regexp2 knows that vocabulary - RE2 has only general categories and
// scripts under its own spellings, regexp2 a subset of .NET's names, and
// neither has Script_Extensions or most binary properties. So every property
// escape is resolved here (jsregex knows the names and aliases) and spliced
// into the pattern as explicit ranges before either engine parses it.
//
// The sets themselves come from regex_unicode_data.go, generated from a
// reference engine at the same Unicode version as Go's unicode package.

// runeRange is an inclusive [lo, hi] span of code points.
type runeRange struct{ lo, hi rune }

// unicodeProperty is one property's member spans and their gaps, decoded
// from the generated data on first use. Surrogates are left out of both: a
// Go string can't carry one as a literal rune, and no UTF-8 subject can
// contain one either.
type unicodeProperty struct {
	once       sync.Once
	data       string
	ranges     []runeRange
	complement []runeRange
}

func (p *unicodeProperty) get() *unicodeProperty {
	p.once.Do(func() {
		raw, err := base64.StdEncoding.DecodeString(p.data)
		if err != nil {
			panic("regex_unicode_data: " + err.Error())
		}
		var members []runeRange
		next := rune(0)
		for len(raw) > 0 {
			gap, n := binary.Uvarint(raw)
			raw = raw[n:]
			width, n := binary.Uvarint(raw)
			raw = raw[n:]
			lo := next + rune(gap)
			hi := lo + rune(width)
			members = append(members, runeRange{lo, hi})
			next = hi + 1
		}
		next = 0
		for _, rr := range members {
			if rr.lo > next {
				p.complement = appendWithoutSurrogates(p.complement, next, rr.lo-1)
			}
			p.ranges = appendWithoutSurrogates(p.ranges, rr.lo, rr.hi)
			next = rr.hi + 1
		}
		if next <= unicode.MaxRune {
			p.complement = appendWithoutSurrogates(p.complement, next, unicode.MaxRune)
		}
	})
	return p
}

var unicodeProperties = func() map[string]*unicodeProperty {
	m := make(map[string]*unicodeProperty, len(generatedUnicodeSets))
	for k, v := range generatedUnicodeSets {
		m[k] = &unicodeProperty{data: v}
	}
	return m
}()

// lookupUnicodeProperty returns the code point set a \p{...} body names, or
// nil when it names no set of code points (an invalid name, or a property
// of strings).
func lookupUnicodeProperty(body string) *unicodeProperty {
	name, value, hasValue := strings.Cut(body, "=")
	kind, canonical := jsregex.ResolveProperty(name, value, hasValue)
	var key string
	switch kind {
	case jsregex.PropertyGeneralCategory:
		key = "gc=" + canonical
	case jsregex.PropertyScript:
		key = "sc=" + canonical
	case jsregex.PropertyScriptExtensions:
		key = "scx=" + canonical
	case jsregex.PropertyBinary:
		key = canonical
	default:
		return nil
	}
	if p := unicodeProperties[key]; p != nil {
		return p.get()
	}
	return nil
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

// expandUnicodePropertyEscapes rewrites every property escape for the
// engines. Under the u and v flags each \p{...} / \P{...} naming a set of
// code points becomes its ranges (or their complement) - only the members
// inside a character class, wrapped in a class of their own outside one - and
// a standalone \p{...} naming a property of strings becomes its
// sequence-alternation expansion (see regex_emoji.go). Without those flags
// \p and \P are identity escapes (Annex B), so the backslash is dropped and
// the letter stays literal. Any escape left untouched is for the engines to
// judge; the pattern has already been validated.
//
// Operates on bytes like rewriteECMAClasses: every construct inspected is
// ASCII, so multi-byte runes copy through as-is.
func expandUnicodePropertyEscapes(pattern, flags string) string {
	if !strings.Contains(pattern, `\p`) && !strings.Contains(pattern, `\P`) {
		return pattern
	}
	unicodeMode := strings.ContainsAny(flags, "uv")
	var b strings.Builder
	b.Grow(len(pattern))
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '\\' && i+1 < len(pattern) {
			e := pattern[i+1]
			if (e == 'p' || e == 'P') && !unicodeMode {
				b.WriteByte(e)
				i++
				continue
			}
			if (e == 'p' || e == 'P') && i+2 < len(pattern) && pattern[i+2] == '{' {
				if end := strings.IndexByte(pattern[i+3:], '}'); end >= 0 {
					body := pattern[i+3 : i+3+end]
					if prop := lookupUnicodeProperty(body); prop != nil {
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
					// A property of strings is only valid as \p{Name}, and a
					// multi-codepoint alternation has no equivalent inside
					// `[...]` in either engine, so only the standalone form
					// is expanded.
					if e == 'p' && !inClass {
						if v := stringPropertyValue(body); v != nil {
							b.WriteString(v.pattern())
							i += 3 + end
							continue
						}
					}
				}
			}
			// Any other escape passes through with its operand, which also
			// keeps \\ and \[ from being read as syntax below.
			b.WriteByte(c)
			b.WriteByte(e)
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

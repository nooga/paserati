package vm

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/nooga/paserati/pkg/jsregex"
)

// UnicodeSets mode (the v flag) gives character classes set operations -
// nested classes, intersection (&&), subtraction (--) - and string members
// (\q{...}, properties of strings). Neither engine has any of that, so each
// top-level v-mode class is evaluated here to the set it denotes and spelled
// as a plain class, or as an alternation of its strings (longest first, as
// the spec orders them) ahead of a plain class for its single characters.

// charSet is one bit per code point.
type charSet []uint64

func newCharSet() charSet { return make(charSet, (unicode.MaxRune+1+63)/64) }

func (s charSet) has(r rune) bool { return s[r>>6]&(1<<(uint(r)&63)) != 0 }
func (s charSet) add(r rune)      { s[r>>6] |= 1 << (uint(r) & 63) }

func (s charSet) addRange(lo, hi rune) {
	for r := lo; r <= hi; r++ {
		s.add(r)
	}
}

func (s charSet) addRanges(rs []runeRange) {
	for _, rr := range rs {
		s.addRange(rr.lo, rr.hi)
	}
}

func (s charSet) union(o charSet) {
	for i := range s {
		s[i] |= o[i]
	}
}

func (s charSet) intersect(o charSet) {
	for i := range s {
		s[i] &= o[i]
	}
}

func (s charSet) subtract(o charSet) {
	for i := range s {
		s[i] &^= o[i]
	}
}

func (s charSet) complement() {
	for i := range s {
		s[i] = ^s[i]
	}
}

// ranges lists the members as spans, leaving out surrogates (see
// unicodeProperty).
func (s charSet) ranges() []runeRange {
	var out []runeRange
	start := rune(-1)
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if start < 0 && s[r>>6] == 0 {
			r |= 63
			continue
		}
		if s.has(r) {
			if start < 0 {
				start = r
			}
		} else if start >= 0 {
			out = appendWithoutSurrogates(out, start, r-1)
			start = -1
		}
	}
	if start >= 0 {
		out = appendWithoutSurrogates(out, start, unicode.MaxRune)
	}
	return out
}

// caseOrbits lists every code point whose simple case folding orbit has more
// than one member, each with its orbit.
var caseOrbits = sync.OnceValue(func() map[rune][]rune {
	m := map[rune][]rune{}
	for r := rune(0); r <= unicode.MaxRune; r++ {
		if f := unicode.SimpleFold(r); f != r {
			orbit := []rune{r}
			for f != r {
				orbit = append(orbit, f)
				f = unicode.SimpleFold(f)
			}
			m[r] = orbit
		}
	}
	return m
})

// closeUnderCaseFolding adds every case variant of every member: with the i
// flag each operand is taken through simple case folding before the set
// operations (MaybeSimpleCaseFolding), and a set closed under folding
// behaves the same under the engines' own case-insensitive matching.
func (s charSet) closeUnderCaseFolding() {
	for r, orbit := range caseOrbits() {
		if s.has(r) {
			for _, o := range orbit {
				s.add(o)
			}
		}
	}
}

// classValue is what a v-mode class denotes.
type classValue struct {
	chars  charSet
	strs   map[string]bool // members of length other than one code point
	pieces []string        // properties of strings, as pattern fragments; union only
}

func (v *classValue) addString(rs []rune) {
	if len(rs) == 1 {
		v.chars.add(rs[0])
		return
	}
	if v.strs == nil {
		v.strs = map[string]bool{}
	}
	v.strs[string(rs)] = true
}

var errClassSetUnsupported = fmt.Errorf("UnicodeSets mode: properties of strings cannot be intersected or subtracted")

func evalClassSet(set *jsregex.ClassSet, ignoreCase bool) (*classValue, error) {
	var acc *classValue
	for i, item := range set.Items {
		v, err := evalClassSetItem(item, ignoreCase)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			acc = v
			continue
		}
		switch set.Op {
		case jsregex.ClassUnion:
			acc.chars.union(v.chars)
			for s := range v.strs {
				acc.addString([]rune(s))
			}
			acc.pieces = append(acc.pieces, v.pieces...)
		case jsregex.ClassIntersection, jsregex.ClassSubtraction:
			if acc.pieces != nil || v.pieces != nil {
				return nil, errClassSetUnsupported
			}
			if set.Op == jsregex.ClassIntersection {
				acc.chars.intersect(v.chars)
				for s := range acc.strs {
					if !v.strs[s] {
						delete(acc.strs, s)
					}
				}
			} else {
				acc.chars.subtract(v.chars)
				for s := range v.strs {
					delete(acc.strs, s)
				}
			}
		}
	}
	if acc == nil {
		acc = &classValue{chars: newCharSet()}
	}
	if set.Negated {
		// The parser guarantees a negated class holds no strings.
		acc.chars.complement()
	}
	return acc, nil
}

func evalClassSetItem(item jsregex.ClassSetItem, ignoreCase bool) (*classValue, error) {
	v := &classValue{chars: newCharSet()}
	switch item.Kind {
	case jsregex.ClassSetChar:
		v.chars.add(item.Lo)
	case jsregex.ClassSetRange:
		v.chars.addRange(item.Lo, item.Hi)
	case jsregex.ClassSetNested:
		return evalClassSet(item.Nested, ignoreCase)
	case jsregex.ClassSetStrings:
		for _, s := range item.Strings {
			v.addString(s)
		}
	case jsregex.ClassSetEscape:
		switch unicode.ToLower(item.Escape) {
		case 'd':
			v.chars.addRange('0', '9')
		case 's':
			for _, r := range []rune{'\t', '\n', '\v', '\f', '\r', 0x2028, 0x2029, 0xFEFF} {
				v.chars.add(r)
			}
			v.chars.addRanges(lookupUnicodeProperty("Space_Separator").ranges)
		case 'w':
			v.chars.addRange('a', 'z')
			v.chars.addRange('A', 'Z')
			v.chars.addRange('0', '9')
			v.chars.add('_')
			if ignoreCase {
				// WordCharacters under u/v with i: whatever folds into it.
				v.chars.closeUnderCaseFolding()
			}
		}
		if unicode.IsUpper(item.Escape) {
			v.chars.complement()
		}
		return v, nil
	case jsregex.ClassSetProperty:
		if prop := lookupUnicodeProperty(item.Property); prop != nil {
			if item.Escape == 'P' {
				v.chars.addRanges(prop.complement)
			} else {
				v.chars.addRanges(prop.ranges)
			}
		} else if sv := stringPropertyValue(item.Property); sv != nil {
			return sv, nil
		} else {
			return nil, fmt.Errorf("unknown property %q", item.Property)
		}
	}
	if ignoreCase {
		v.chars.closeUnderCaseFolding()
	}
	return v, nil
}

// rewriteClassSet is the jsregex.ClassSetRewriter the VM compiles v-mode
// patterns with.
func rewriteClassSet(ignoreCase bool) jsregex.ClassSetRewriter {
	return func(set *jsregex.ClassSet) (string, error) {
		v, err := evalClassSet(set, ignoreCase)
		if err != nil {
			return "", err
		}
		return v.pattern(), nil
	}
}

// pattern spells the value for the engines: a plain class when it holds
// only single code points, otherwise an alternation of its strings, longest
// first, then the class, then the empty string if it is a member.
func (v *classValue) pattern() string {
	ranges := v.chars.ranges()
	if len(v.strs) == 0 && len(v.pieces) == 0 {
		if len(ranges) == 0 {
			// The empty class, which matches nothing.
			return `[^\x00-` + string(rune(unicode.MaxRune)) + `]`
		}
		var b strings.Builder
		writeRangesClass(&b, ranges)
		return b.String()
	}

	strs := make([]string, 0, len(v.strs))
	for s := range v.strs {
		if s != "" {
			strs = append(strs, s)
		}
	}
	sort.Slice(strs, func(i, j int) bool {
		li, lj := utf8.RuneCountInString(strs[i]), utf8.RuneCountInString(strs[j])
		if li != lj {
			return li > lj
		}
		return strs[i] < strs[j]
	})
	branches := append([]string(nil), v.pieces...)
	for _, s := range strs {
		var sb strings.Builder
		for _, r := range s {
			writeSetRune(&sb, r)
		}
		branches = append(branches, sb.String())
	}
	if len(ranges) > 0 {
		var b strings.Builder
		writeRangesClass(&b, ranges)
		branches = append(branches, b.String())
	}
	if v.strs[""] {
		branches = append(branches, "")
	}
	return "(?:" + strings.Join(branches, "|") + ")"
}

// writeSetRune spells one code point so that it survives the later
// translation passes and means itself inside or outside a class: ASCII
// punctuation escaped with a backslash, everything else literal.
func writeSetRune(b *strings.Builder, r rune) {
	if r < 0x80 && (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r > ' ' && r != 0x7F {
		b.WriteByte('\\')
	}
	b.WriteRune(r)
}

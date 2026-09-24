// Package jsregex checks ECMAScript regular expression patterns and flags
// against the Pattern grammar and its early errors (ECMA-262 §22.2.1, with the
// Annex B.1.2 extensions that apply outside Unicode mode).
//
// It is the single source of truth for "is this a valid RegExp": the parser
// runs it on every regular expression literal, so an invalid literal is a
// SyntaxError before any code runs, and the RegExp constructor runs it on its
// arguments, so `new RegExp(p, f)` rejects exactly the same patterns at run
// time. The engines that later execute a pattern never have to be trusted to
// enforce ECMAScript's syntax rules.
package jsregex

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Error is a pattern or flags syntax error.
type Error struct {
	Pattern string
	Flags   string
	Reason  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Invalid regular expression: /%s/%s: %s", e.Pattern, e.Flags, e.Reason)
}

// ValidateFlags checks that every flag is one of d g i m s u v y, that none
// repeats, and that u and v (Unicode mode and UnicodeSets mode) are not both
// present.
func ValidateFlags(flags string) error {
	var seen [128]bool
	for _, f := range flags {
		if f >= 128 || !strings.ContainsRune("dgimsuvy", f) || seen[f] {
			return fmt.Errorf("Invalid regular expression flags '%s'", flags)
		}
		seen[f] = true
	}
	if seen['u'] && seen['v'] {
		return fmt.Errorf("Invalid regular expression flags '%s'", flags)
	}
	return nil
}

// Validate reports whether pattern is a valid Pattern under flags, returning
// an *Error describing the first problem found. Flags are validated first.
func Validate(pattern, flags string) error {
	_, err := Analyze(pattern, flags)
	return err
}

// Pattern is what the validating parse learns about a valid pattern.
type Pattern struct {
	// CaptureNames holds each capture group's name by capture number, ""
	// for an unnamed group; element 0 stands for the whole match. It is nil
	// when the pattern has no named group.
	CaptureNames []string
	// CaptureCount is the number of capture groups.
	CaptureCount int
	// Engine is the pattern respelled for a backtracking engine that knows
	// neither ECMAScript's group-name syntax nor its Annex B escapes: every
	// named group is a plain capturing group (so engines number captures in
	// source order), every \k<name> a backreference to the groups of that
	// name, and every character escape spelled with a letter or digit (hex,
	// Unicode, control, legacy octal and identity escapes) the character it
	// denotes. Under the v flag each class is replaced by what the
	// ClassSetRewriter makes of it.
	Engine string
}

// Analyze validates pattern under flags and describes it.
func Analyze(pattern, flags string) (*Pattern, error) {
	return AnalyzeForEngine(pattern, flags, nil)
}

// ClassSetRewriter spells a UnicodeSets-mode class for an engine that has no
// class set operations.
type ClassSetRewriter func(*ClassSet) (string, error)

// AnalyzeForEngine is Analyze, with every top-level UnicodeSets-mode class
// in the engine pattern replaced by what rewrite makes of it. A rewrite
// error is not a SyntaxError: the pattern is valid, the engine just can't
// run it.
func AnalyzeForEngine(pattern, flags string, rewrite ClassSetRewriter) (*Pattern, error) {
	if err := ValidateFlags(flags); err != nil {
		return nil, err
	}
	v := strings.ContainsRune(flags, 'v')
	u := v || strings.ContainsRune(flags, 'u')
	src, offs := decodePattern(pattern, u)
	info := prescan(src, v)

	// Annex B: outside Unicode mode a pattern is first read without named
	// groups, and only re-read with them when it declares one. In Unicode mode
	// named groups are always on.
	p := &parser{
		src:        src,
		offs:       offs,
		u:          u,
		v:          v,
		named:      u || info.hasNamedGroup,
		groupCount: info.groupCount,
		captures:   []string{""},
	}
	if reason := p.parse(); reason != "" {
		return nil, &Error{Pattern: pattern, Flags: flags, Reason: reason}
	}
	engine, err := p.enginePattern(pattern, rewrite)
	if err != nil {
		return nil, err
	}
	res := &Pattern{CaptureCount: len(p.captures) - 1, Engine: engine}
	if len(p.names) > 0 {
		res.CaptureNames = p.captures
	}
	return res, nil
}

// decodePattern turns the pattern text into the units the grammar works on:
// UTF-16 code units outside Unicode mode (so an astral character is two
// pattern characters, as it is in any other engine), code points inside it.
// The input may be WTF-8, carrying lone surrogates as their 3-byte forms.
// offs[i] is the byte offset of unit i, with a final entry for the end.
func decodePattern(s string, unicodeMode bool) ([]rune, []int) {
	out := make([]rune, 0, len(s))
	offs := make([]int, 0, len(s)+1)
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			out = append(out, rune(c))
			offs = append(offs, i)
			i++
			continue
		}
		// A surrogate half encoded on its own: ED A0..BF xx.
		if c == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF {
			out = append(out, rune(c&0x0F)<<12|rune(s[i+1]&0x3F)<<6|rune(s[i+2]&0x3F))
			offs = append(offs, i)
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r > 0xFFFF && !unicodeMode {
			r -= 0x10000
			out = append(out, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
			offs = append(offs, i, i)
		} else {
			out = append(out, r)
			offs = append(offs, i)
		}
		i += size
	}
	offs = append(offs, len(s))
	if unicodeMode {
		// Join a lead/trail pair that arrived as two separate 3-byte forms.
		j := 0
		for i := 0; i < len(out); i++ {
			offs[j] = offs[i]
			if isLead(out[i]) && i+1 < len(out) && isTrail(out[i+1]) {
				out[j] = combine(out[i], out[i+1])
				i++
			} else {
				out[j] = out[i]
			}
			j++
		}
		offs[j] = len(s)
		out, offs = out[:j], offs[:j+1]
	}
	return out, offs
}

func isLead(r rune) bool  { return r >= 0xD800 && r <= 0xDBFF }
func isTrail(r rune) bool { return r >= 0xDC00 && r <= 0xDFFF }
func combine(lead, trail rune) rune {
	return 0x10000 + (lead-0xD800)<<10 + (trail - 0xDC00)
}

type prescanInfo struct {
	groupCount    int
	hasNamedGroup bool
}

// prescan counts capturing groups (CountLeftCapturingParens) and notes
// whether any group is named. Both are needed before the real parse: a
// decimal escape's meaning depends on the total group count, and outside
// Unicode mode \k's meaning depends on whether any named group exists.
func prescan(src []rune, v bool) prescanInfo {
	var info prescanInfo
	classDepth := 0
	for i := 0; i < len(src); i++ {
		switch c := src[i]; {
		case c == '\\':
			i++
		case c == '[':
			if classDepth == 0 || v {
				classDepth++
			}
		case c == ']':
			if classDepth > 0 {
				classDepth--
			}
		case c == '(' && classDepth == 0:
			if i+1 < len(src) && src[i+1] == '?' {
				if i+2 < len(src) && src[i+2] == '<' && i+3 < len(src) && src[i+3] != '=' && src[i+3] != '!' {
					info.groupCount++
					info.hasNamedGroup = true
				}
			} else {
				info.groupCount++
			}
		}
	}
	return info
}

// groupName is one GroupSpecifier's name, with the chain of alternatives it
// sits in: one (disjunction, alternative) pair per enclosing Disjunction.
type groupName struct {
	name string
	path []altPos
}

type altPos struct{ disj, alt int }

// groupRef is a \k<name> spanning src units [start, end).
type groupRef struct {
	name       string
	start, end int
}

// namedGroup is a (?<name>...) group: its '(' at open, the '>' ending the
// name just before nameEnd, its ')' at close.
type namedGroup struct {
	name                 string
	open, nameEnd, close int
}

// classSpan is a top-level v-mode class spanning src units [start, end).
type classSpan struct {
	start, end int
	set        *ClassSet
}

// edit replaces src units [start, end) with text in the engine pattern.
type edit struct {
	start, end int
	text       string
}

// enginePattern applies the recorded edits to the original pattern text.
func (p *parser) enginePattern(pattern string, rewrite ClassSetRewriter) (string, error) {
	if rewrite != nil {
		for _, c := range p.classSets {
			text, err := rewrite(c.set)
			if err != nil {
				return "", err
			}
			p.edits = append(p.edits, edit{c.start, c.end, text})
		}
	}
	// A backreference to a name several groups share must follow whichever
	// of them captured last, which a regexp2 backreference by number can't
	// express: it keeps a group's capture from an earlier iteration of an
	// enclosing quantifier, where ECMAScript clears it. So each of those
	// groups also gets an inner group under one shared engine name, and the
	// reference names that. Named engine groups are numbered after every
	// unnamed one, so the ECMAScript numbering of the outer groups holds.
	shared := map[string]string{}
	for _, ref := range p.refs {
		count := 0
		for _, name := range p.captures {
			if name == ref.name {
				count++
			}
		}
		if count > 1 && shared[ref.name] == "" {
			shared[ref.name] = fmt.Sprintf("js%d", len(shared)+1)
		}
	}
	for _, g := range p.namedGroups {
		if engineName := shared[g.name]; engineName != "" {
			p.edits = append(p.edits, edit{g.open, g.nameEnd, "((?<" + engineName + ">"},
				edit{g.close, g.close, ")"})
		} else {
			p.edits = append(p.edits, edit{g.open, g.nameEnd, "("})
		}
	}
	for _, ref := range p.refs {
		if engineName := shared[ref.name]; engineName != "" {
			p.edits = append(p.edits, edit{ref.start, ref.end, `\k<` + engineName + ">"})
			continue
		}
		for n, name := range p.captures {
			if n > 0 && name == ref.name {
				p.edits = append(p.edits, edit{ref.start, ref.end, fmt.Sprintf("(?:\\%d)", n)})
			}
		}
	}
	if len(p.edits) == 0 {
		return pattern, nil
	}
	sort.SliceStable(p.edits, func(i, j int) bool { return p.edits[i].start < p.edits[j].start })
	var b strings.Builder
	last := 0
	for _, e := range p.edits {
		b.WriteString(pattern[last:p.offs[e.start]])
		b.WriteString(e.text)
		last = p.offs[e.end]
	}
	b.WriteString(pattern[last:])
	return b.String(), nil
}

type parser struct {
	src        []rune
	offs       []int // byte offset of each src unit in the pattern text
	pos        int
	u, v       bool // UnicodeMode, UnicodeSetsMode
	named      bool // NamedCaptureGroups
	groupCount int

	names     []groupName
	refs      []groupRef  // \k<name> references, checked once every group is known
	captures  []string    // capture names by number, "" when unnamed
	edits     []edit      // respellings for the engine pattern
	classSets []classSpan // top-level UnicodeSets-mode classes

	namedGroups []namedGroup
	inClassSet  bool
	altPath     []altPos
	nextDisj    int
}

func (p *parser) parse() string {
	if reason := p.disjunction(); reason != "" {
		return reason
	}
	if p.pos < len(p.src) {
		if p.src[p.pos] == ')' {
			return "Unmatched ')'"
		}
		return "Unexpected character"
	}
	for _, ref := range p.refs {
		found := false
		for _, g := range p.names {
			if g.name == ref.name {
				found = true
				break
			}
		}
		if !found {
			return "Invalid named capture referenced"
		}
	}
	return ""
}

func (p *parser) eof() bool { return p.pos >= len(p.src) }

func (p *parser) peek() rune {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return -1
}

func (p *parser) peekAt(off int) rune {
	if p.pos+off < len(p.src) {
		return p.src[p.pos+off]
	}
	return -1
}

func (p *parser) eat(c rune) bool {
	if p.peek() == c {
		p.pos++
		return true
	}
	return false
}

func (p *parser) lookingAt(s string) bool {
	i := 0
	for _, c := range s {
		if p.peekAt(i) != c {
			return false
		}
		i++
	}
	return true
}

// disjunction parses Alternative ( | Alternative )* up to an unmatched ')'
// or the end of input.
func (p *parser) disjunction() string {
	disj := p.nextDisj
	p.nextDisj++
	alt := 0
	for {
		p.altPath = append(p.altPath, altPos{disj, alt})
		reason := p.alternative()
		p.altPath = p.altPath[:len(p.altPath)-1]
		if reason != "" {
			return reason
		}
		if !p.eat('|') {
			return ""
		}
		alt++
	}
}

func (p *parser) alternative() string {
	for !p.eof() && p.peek() != '|' && p.peek() != ')' {
		if reason := p.term(); reason != "" {
			return reason
		}
	}
	return ""
}

func (p *parser) term() string {
	c := p.peek()
	switch c {
	case '^', '$':
		p.pos++
		return p.noQuantifier()
	case '\\':
		if n := p.peekAt(1); n == 'b' || n == 'B' {
			p.pos += 2
			return p.noQuantifier()
		}
	case '(':
		if p.lookingAt("(?=") || p.lookingAt("(?!") {
			p.pos += 3
			if reason := p.groupBody(); reason != "" {
				return reason
			}
			// Annex B: a lookahead is quantifiable outside Unicode mode.
			if p.u {
				return p.noQuantifier()
			}
			return p.optionalQuantifier()
		}
		if p.lookingAt("(?<=") || p.lookingAt("(?<!") {
			p.pos += 4
			if reason := p.groupBody(); reason != "" {
				return reason
			}
			return p.noQuantifier()
		}
	}
	if reason := p.atom(); reason != "" {
		return reason
	}
	return p.optionalQuantifier()
}

// noQuantifier rejects a quantifier following a non-quantifiable term.
func (p *parser) noQuantifier() string {
	switch p.peek() {
	case '*', '+', '?':
		return "Nothing to repeat"
	case '{':
		if ok, _ := p.bracedQuantifier(); ok || p.u {
			return "Nothing to repeat"
		}
	}
	return ""
}

func (p *parser) optionalQuantifier() string {
	switch p.peek() {
	case '*', '+', '?':
		p.pos++
	case '{':
		ok, reason := p.bracedQuantifier()
		if reason != "" {
			return reason
		}
		if !ok {
			if p.u {
				return "Incomplete quantifier"
			}
			// Annex B: a '{' that starts no quantifier is a literal.
			return ""
		}
	default:
		return ""
	}
	p.eat('?')
	return ""
}

// bracedQuantifier consumes {n}, {n,} or {n,m} when one starts at the cursor.
// It reports whether one did, and a reason when it did but is out of order.
func (p *parser) bracedQuantifier() (bool, string) {
	start := p.pos
	p.pos++ // '{'
	lo := p.digits()
	if lo == "" {
		p.pos = start
		return false, ""
	}
	hi := lo
	if p.eat(',') {
		hi = p.digits()
	}
	if !p.eat('}') {
		p.pos = start
		return false, ""
	}
	if hi != "" && compareDecimal(lo, hi) > 0 {
		return true, "numbers out of order in {} quantifier"
	}
	return true, ""
}

func (p *parser) digits() string {
	start := p.pos
	for isDecimal(p.peek()) {
		p.pos++
	}
	return string(p.src[start:p.pos])
}

// compareDecimal compares two unsigned decimal strings of any length.
func compareDecimal(a, b string) int {
	a = strings.TrimLeft(a, "0")
	b = strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func isDecimal(c rune) bool { return c >= '0' && c <= '9' }
func isOctal(c rune) bool   { return c >= '0' && c <= '7' }
func isHex(c rune) bool {
	return isDecimal(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
func isAsciiLetter(c rune) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func hexValue(c rune) rune {
	switch {
	case isDecimal(c):
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

func isSyntaxCharacter(c rune) bool {
	return c >= 0 && c < 128 && strings.ContainsRune(`^$\.*+?()[]{}|`, c)
}

func (p *parser) atom() string {
	c := p.peek()
	switch c {
	case '.':
		p.pos++
		return ""
	case '(':
		return p.group()
	case '[':
		p.pos++
		if p.v {
			start := p.pos - 1
			// The whole class is respelled at once; nothing inside it is.
			p.inClassSet = true
			set, _, reason := p.classSetContents()
			p.inClassSet = false
			if reason == "" {
				p.classSets = append(p.classSets, classSpan{start, p.pos, set})
			}
			return reason
		}
		return p.classRanges()
	case '\\':
		p.pos++
		return p.atomEscape()
	case '*', '+', '?':
		return "Nothing to repeat"
	case '{':
		if p.u {
			return "Lone quantifier brackets"
		}
		// Annex B: an ExtendedAtom may not be a well-formed braced
		// quantifier, but any other '{' is a literal.
		if ok, _ := p.bracedQuantifier(); ok {
			return "Nothing to repeat"
		}
		p.pos++
		return ""
	case '}', ']':
		if p.u {
			return "Lone quantifier brackets"
		}
		p.pos++
		return ""
	}
	p.pos++
	return ""
}

func (p *parser) group() string {
	open := p.pos
	p.pos++ // '('
	if !p.eat('?') {
		p.captures = append(p.captures, "")
		return p.groupBody()
	}
	switch p.peek() {
	case ':':
		p.pos++
		return p.groupBody()
	case '<':
		p.pos++
		name, reason := p.groupNameBody()
		if reason != "" {
			return reason
		}
		for _, g := range p.names {
			if g.name == name && mightBothParticipate(g.path, p.altPath) {
				return "Duplicate capture group name"
			}
		}
		p.names = append(p.names, groupName{name: name, path: append([]altPos(nil), p.altPath...)})
		p.captures = append(p.captures, name)
		g := namedGroup{name: name, open: open, nameEnd: p.pos}
		if reason := p.groupBody(); reason != "" {
			return reason
		}
		g.close = p.pos - 1
		p.namedGroups = append(p.namedGroups, g)
		return ""
	}
	return p.modifiers()
}

// mightBothParticipate is false only when the two groups sit in different
// alternatives of some Disjunction that encloses both.
func mightBothParticipate(a, b []altPos) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].disj != b[i].disj {
			break
		}
		if a[i].alt != b[i].alt {
			return false
		}
	}
	return true
}

// modifiers parses the `ims-ims:` part of a modifier group; `(?` has been
// consumed.
func (p *parser) modifiers() string {
	add, ok := p.modifierRun()
	if !ok {
		return "Invalid group"
	}
	remove := ""
	hasDash := p.eat('-')
	if hasDash {
		if remove, ok = p.modifierRun(); !ok {
			return "Invalid group"
		}
	}
	if !p.eat(':') {
		return "Invalid group"
	}
	if hasDash && add == "" && remove == "" {
		return "Invalid regular expression modifiers"
	}
	for _, m := range add {
		if strings.ContainsRune(remove, m) {
			return "Invalid regular expression modifiers"
		}
	}
	return p.groupBody()
}

// modifierRun reads RegularExpressionModifiers: letters from i, m, s with
// no repeats.
func (p *parser) modifierRun() (string, bool) {
	var run []rune
	for {
		c := p.peek()
		if c != 'i' && c != 'm' && c != 's' {
			if c != '-' && c != ':' {
				return "", false
			}
			return string(run), true
		}
		for _, r := range run {
			if r == c {
				return "", false
			}
		}
		run = append(run, c)
		p.pos++
	}
}

// groupBody parses Disjunction ')'.
func (p *parser) groupBody() string {
	if reason := p.disjunction(); reason != "" {
		return reason
	}
	if !p.eat(')') {
		return "Unterminated group"
	}
	return ""
}

// groupNameBody parses RegExpIdentifierName '>'; '<' has been consumed.
func (p *parser) groupNameBody() (string, string) {
	var name []rune
	for {
		if p.eat('>') {
			if len(name) == 0 {
				return "", "Invalid capture group name"
			}
			return string(name), ""
		}
		if p.eof() {
			return "", "Invalid capture group name"
		}
		c := p.src[p.pos]
		p.pos++
		if c == '\\' {
			// Escapes in a group name always use the Unicode-mode forms.
			if !p.eat('u') {
				return "", "Invalid capture group name"
			}
			r, ok := p.unicodeEscapeBody(true)
			if !ok {
				return "", "Invalid Unicode escape"
			}
			c = r
		} else if isLead(c) && isTrail(p.peek()) {
			c = combine(c, p.src[p.pos])
			p.pos++
		}
		if len(name) == 0 {
			if !isIDStart(c) {
				return "", "Invalid capture group name"
			}
		} else if !isIDContinue(c) {
			return "", "Invalid capture group name"
		}
		name = append(name, c)
	}
}

func isIDStart(c rune) bool {
	return c == '$' || c == '_' || unicode.In(c, unicode.L, unicode.Nl, unicode.Other_ID_Start) &&
		!unicode.In(c, unicode.Pattern_Syntax, unicode.Pattern_White_Space)
}

func isIDContinue(c rune) bool {
	return c == '$' || c == 0x200C || c == 0x200D || isIDStart(c) ||
		unicode.In(c, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc, unicode.Other_ID_Continue) &&
			!unicode.In(c, unicode.Pattern_Syntax, unicode.Pattern_White_Space)
}

// unicodeEscapeBody parses what follows `\u`: Hex4Digits, and in Unicode mode
// also {CodePoint} and an escaped surrogate pair \uLead\uTrail. The cursor is
// left untouched on failure.
func (p *parser) unicodeEscapeBody(unicodeMode bool) (rune, bool) {
	start := p.pos
	if unicodeMode && p.eat('{') {
		var v rune
		n := 0
		for isHex(p.peek()) {
			v = v*16 + hexValue(p.peek())
			if v > unicode.MaxRune {
				p.pos = start
				return 0, false
			}
			p.pos++
			n++
		}
		if n == 0 || !p.eat('}') {
			p.pos = start
			return 0, false
		}
		return v, true
	}
	v, ok := p.hex4()
	if !ok {
		p.pos = start
		return 0, false
	}
	if unicodeMode && isLead(v) && p.peek() == '\\' && p.peekAt(1) == 'u' {
		save := p.pos
		p.pos += 2
		if t, ok := p.hex4(); ok && isTrail(t) {
			return combine(v, t), true
		}
		p.pos = save
	}
	return v, true
}

func (p *parser) hex4() (rune, bool) {
	var v rune
	for i := 0; i < 4; i++ {
		c := p.peekAt(i)
		if !isHex(c) {
			return 0, false
		}
		v = v*16 + hexValue(c)
	}
	p.pos += 4
	return v, true
}

// atomEscape parses what follows a '\' outside a character class.
func (p *parser) atomEscape() string {
	if p.eof() {
		return "\\ at end of pattern"
	}
	c := p.peek()
	switch {
	case c >= '1' && c <= '9':
		start := p.pos
		n := p.digits()
		if p.u {
			if compareDecimal(n, fmt.Sprint(p.groupCount)) > 0 {
				return "Invalid escape"
			}
			return ""
		}
		if compareDecimal(n, fmt.Sprint(p.groupCount)) <= 0 {
			return ""
		}
		// Annex B: a decimal escape naming no group is a legacy octal
		// escape, or an identity escape for 8 and 9.
		p.pos = start
		_, _, reason := p.characterEscape()
		return reason
	case c == 'k':
		if !p.named {
			p.pos++
			p.edits = append(p.edits, edit{p.pos - 2, p.pos, "k"})
			return ""
		}
		start := p.pos - 1
		p.pos++
		if !p.eat('<') {
			return "Invalid named reference"
		}
		name, reason := p.groupNameBody()
		if reason != "" {
			return reason
		}
		p.refs = append(p.refs, groupRef{name, start, p.pos})
		return ""
	case c == 'c' && !p.u && !isAsciiLetter(p.peekAt(1)):
		// Annex B: `\` [lookahead = c] is the backslash itself; the 'c'
		// is read as the next atom.
		p.edits = append(p.edits, edit{p.pos - 1, p.pos, `\\`})
		return ""
	}
	if isClass, _, reason := p.characterClassEscape(); isClass || reason != "" {
		return reason
	}
	_, _, reason := p.characterEscape()
	return reason
}

// characterClassEscape parses d D s S w W and, in Unicode mode, p{...} and
// P{...}. It reports whether one was present and whether the property may
// match strings.
func (p *parser) characterClassEscape() (bool, bool, string) {
	switch p.peek() {
	case 'd', 'D', 's', 'S', 'w', 'W':
		p.pos++
		return true, false, ""
	case 'p', 'P':
		if !p.u {
			return false, false, ""
		}
		negated := p.peek() == 'P'
		p.pos++
		strs, reason := p.propertyExpression(negated)
		return true, strs, reason
	}
	return false, false, ""
}

// propertyExpression parses `{ UnicodePropertyValueExpression }` after \p or \P.
func (p *parser) propertyExpression(negated bool) (bool, string) {
	if !p.eat('{') {
		return false, "Invalid property name"
	}
	start := p.pos
	for !p.eof() && p.peek() != '}' {
		p.pos++
	}
	if p.eof() {
		return false, "Invalid property name"
	}
	body := string(p.src[start:p.pos])
	p.pos++ // '}'
	name, value, hasValue := strings.Cut(body, "=")
	if !isPropertyNameChars(name) || (hasValue && !isPropertyValueChars(value)) {
		return false, "Invalid property name"
	}
	if hasValue {
		if !IsValidPropertyValue(name, value) {
			return false, "Invalid property name"
		}
		return false, ""
	}
	if IsValidLoneProperty(name) {
		return false, ""
	}
	if IsPropertyOfStrings(name) {
		if !p.v {
			return false, "Invalid property name"
		}
		if negated {
			return false, "Invalid property name"
		}
		return true, ""
	}
	return false, "Invalid property name"
}

func isPropertyNameChars(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !isAsciiLetter(c) && c != '_' {
			return false
		}
	}
	return true
}

func isPropertyValueChars(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !isAsciiLetter(c) && c != '_' && !isDecimal(c) {
			return false
		}
	}
	return true
}

// characterEscape parses a CharacterEscape (the cursor is past the '\') and
// returns its character value. ok=false with no reason means the text is no
// CharacterEscape at all (only possible for a `\c` the caller handles).
func (p *parser) characterEscape() (rune, bool, string) {
	start, c := p.pos-1, p.peek()
	v, ok, reason := p.characterEscapeBody()
	if ok && reason == "" && c < utf8.RuneSelf && (isAsciiLetter(c) || isDecimal(c)) && !p.inClassSet {
		// Engines read many of these differently or not at all (\cX, \u{...},
		// \u002A becoming a quantifier once decoded, legacy octal, identity
		// escapes like \z or \8), so each is respelled as its character.
		if lit, fine := engineLiteral(v); fine {
			p.edits = append(p.edits, edit{start, p.pos, lit})
		}
	}
	return v, ok, reason
}

// engineLiteral spells a character so that it means itself inside or
// outside a class in both engines: ASCII punctuation escaped, everything
// else literal. A lone surrogate has no spelling in UTF-8 and is refused.
func engineLiteral(r rune) (string, bool) {
	if r >= 0xD800 && r <= 0xDFFF {
		return "", false
	}
	if r < utf8.RuneSelf && r > ' ' && r != 0x7F && !isAsciiLetter(r) && !isDecimal(r) {
		return `\` + string(r), true
	}
	return string(r), true
}

func (p *parser) characterEscapeBody() (rune, bool, string) {
	c := p.peek()
	switch c {
	case 'f':
		p.pos++
		return '\f', true, ""
	case 'n':
		p.pos++
		return '\n', true, ""
	case 'r':
		p.pos++
		return '\r', true, ""
	case 't':
		p.pos++
		return '\t', true, ""
	case 'v':
		p.pos++
		return '\v', true, ""
	case 'c':
		if l := p.peekAt(1); isAsciiLetter(l) {
			p.pos += 2
			return l % 32, true, ""
		}
		if p.u {
			return 0, false, "Invalid unicode escape"
		}
		return 0, false, ""
	case 'x':
		p.pos++
		if isHex(p.peek()) && isHex(p.peekAt(1)) {
			v := hexValue(p.peek())*16 + hexValue(p.peekAt(1))
			p.pos += 2
			return v, true, ""
		}
		if p.u {
			return 0, false, "Invalid escape"
		}
		return 'x', true, ""
	case 'u':
		p.pos++
		if r, ok := p.unicodeEscapeBody(p.u); ok {
			return r, true, ""
		}
		if p.u {
			return 0, false, "Invalid Unicode escape"
		}
		return 'u', true, ""
	case '0':
		if !isDecimal(p.peekAt(1)) {
			p.pos++
			return 0, true, ""
		}
		if p.u {
			return 0, false, "Invalid decimal escape"
		}
	}
	if !p.u && isOctal(c) {
		// LegacyOctalEscapeSequence: up to three octal digits, value <= 0o377.
		v := c - '0'
		p.pos++
		if isOctal(p.peek()) {
			v = v*8 + p.peek() - '0'
			p.pos++
			if c <= '3' && isOctal(p.peek()) {
				v = v*8 + p.peek() - '0'
				p.pos++
			}
		}
		return v, true, ""
	}
	if p.eof() {
		return 0, false, "\\ at end of pattern"
	}
	// IdentityEscape.
	if p.u {
		if isSyntaxCharacter(c) || c == '/' {
			p.pos++
			return c, true, ""
		}
		return 0, false, "Invalid escape"
	}
	if c == 'k' && p.named {
		return 0, false, "Invalid escape"
	}
	p.pos++
	return c, true, ""
}

// classAtom is one end of a ClassRanges range.
type classAtom struct {
	value   rune
	isClass bool // a class escape such as \d, which cannot bound a range
}

// classRanges parses ClassContents up to and including ']' outside
// UnicodeSets mode; '[' has been consumed.
func (p *parser) classRanges() string {
	p.eat('^')
	for {
		if p.eof() {
			return "Unterminated character class"
		}
		if p.eat(']') {
			return ""
		}
		a, reason := p.classAtom()
		if reason != "" {
			return reason
		}
		if p.peek() == '-' && p.peekAt(1) != ']' && p.peekAt(1) != -1 {
			p.pos++
			b, reason := p.classAtom()
			if reason != "" {
				return reason
			}
			if a.isClass || b.isClass {
				if p.u {
					return "Invalid character class"
				}
				// Annex B: a range with a class escape at either end is
				// the union of both ends and '-'.
				continue
			}
			if a.value > b.value {
				return "Range out of order in character class"
			}
		}
	}
}

func (p *parser) classAtom() (classAtom, string) {
	c := p.src[p.pos]
	p.pos++
	if c != '\\' {
		return classAtom{value: c}, ""
	}
	if p.eof() {
		return classAtom{}, "\\ at end of pattern"
	}
	switch e := p.peek(); {
	case e == 'b':
		p.pos++
		return classAtom{value: '\b'}, ""
	case e == '-' && p.u:
		p.pos++
		return classAtom{value: '-'}, ""
	case e == 'c' && !p.u:
		// Annex B: \c followed by a digit or '_' inside a class.
		if l := p.peekAt(1); isDecimal(l) || l == '_' {
			p.pos += 2
			lit, _ := engineLiteral(l % 32)
			p.edits = append(p.edits, edit{p.pos - 3, p.pos, lit})
			return classAtom{value: l % 32}, ""
		}
		if !isAsciiLetter(p.peekAt(1)) {
			// `\` [lookahead = c]: the backslash itself.
			p.edits = append(p.edits, edit{p.pos - 1, p.pos, `\\`})
			return classAtom{value: '\\'}, ""
		}
	case isDecimal(e) && p.u && e != '0':
		return classAtom{}, "Invalid class escape"
	}
	if isClass, _, reason := p.characterClassEscape(); isClass || reason != "" {
		return classAtom{isClass: true}, reason
	}
	v, _, reason := p.characterEscape()
	return classAtom{value: v}, reason
}

// ---- UnicodeSets mode (v flag) ----

func isClassSetSyntaxCharacter(c rune) bool {
	return c >= 0 && c < 128 && strings.ContainsRune(`()[]{}/-\|`, c)
}

func isClassSetReservedDoublePunctuator(c rune) bool {
	return c >= 0 && c < 128 && strings.ContainsRune("&!#$%*+,.:;<=>?@^`~", c)
}

func isClassSetReservedPunctuator(c rune) bool {
	return c >= 0 && c < 128 && strings.ContainsRune("&-!#%,:;<=>@`~", c)
}

// ClassSetOp is how a UnicodeSets-mode class combines its items.
type ClassSetOp int

const (
	ClassUnion ClassSetOp = iota
	ClassIntersection
	ClassSubtraction
)

// ClassSetItemKind says what a ClassSetItem is.
type ClassSetItemKind int

const (
	ClassSetChar     ClassSetItemKind = iota // Lo
	ClassSetRange                            // Lo-Hi
	ClassSetNested                           // Nested
	ClassSetStrings                          // \q{...}: Strings
	ClassSetEscape                           // \d \D \s \S \w \W: Escape
	ClassSetProperty                         // \p{Property} or \P{Property} (Escape 'p'/'P')
)

// ClassSet is a UnicodeSets-mode (v flag) character class: its items,
// combined by Op, and complemented when Negated.
type ClassSet struct {
	Negated bool
	Op      ClassSetOp
	Items   []ClassSetItem
}

// ClassSetItem is one operand of a ClassSet.
type ClassSetItem struct {
	Kind     ClassSetItemKind
	Lo, Hi   rune
	Nested   *ClassSet
	Strings  [][]rune
	Escape   rune
	Property string
}

// classSetContents parses a v-mode CharacterClass or NestedClass after its
// '[' through its ']', returning it and MayContainStrings.
func (p *parser) classSetContents() (*ClassSet, bool, string) {
	set := &ClassSet{Negated: p.eat('^')}
	strs, reason := p.classSetExpression(set)
	if reason != "" {
		return nil, false, reason
	}
	if set.Negated && strs {
		return nil, false, "Negated character class may contain strings"
	}
	return set, strs && !set.Negated, ""
}

// setOperand is one parsed ClassSetOperand; a lone ClassSetCharacter may
// start a range.
type setOperand struct {
	item    ClassSetItem
	strings bool
}

func (p *parser) classSetExpression(set *ClassSet) (bool, string) {
	if p.eat(']') {
		return false, ""
	}
	first, reason := p.classSetOperand()
	if reason != "" {
		return false, reason
	}
	switch {
	case p.lookingAt("&&"):
		set.Op = ClassIntersection
		return p.classSetOperation(set, first, "&&")
	case p.lookingAt("--"):
		set.Op = ClassSubtraction
		return p.classSetOperation(set, first, "--")
	}
	// ClassUnion.
	mayStrings := false
	cur := first
	for {
		if cur.item.Kind == ClassSetChar && p.peek() == '-' && p.peekAt(1) != '-' {
			p.pos++
			hi, reason := p.classSetOperand()
			if reason != "" {
				return false, reason
			}
			if hi.item.Kind != ClassSetChar {
				return false, "Invalid character class"
			}
			if cur.item.Lo > hi.item.Lo {
				return false, "Range out of order in character class"
			}
			cur.item = ClassSetItem{Kind: ClassSetRange, Lo: cur.item.Lo, Hi: hi.item.Lo}
		} else if cur.strings {
			mayStrings = true
		}
		set.Items = append(set.Items, cur.item)
		if p.eof() {
			return false, "Unterminated character class"
		}
		if p.eat(']') {
			return mayStrings, ""
		}
		if p.lookingAt("&&") || p.lookingAt("--") {
			return false, "Invalid set operation in character class"
		}
		if cur, reason = p.classSetOperand(); reason != "" {
			return false, reason
		}
	}
}

// classSetOperation parses the rest of a ClassIntersection or
// ClassSubtraction whose first operand has been read.
func (p *parser) classSetOperation(set *ClassSet, first setOperand, op string) (bool, string) {
	set.Items = append(set.Items, first.item)
	mayStrings := first.strings
	for {
		if !p.lookingAt(op) {
			if p.eat(']') {
				return mayStrings, ""
			}
			if p.eof() {
				return false, "Unterminated character class"
			}
			return false, "Invalid set operation in character class"
		}
		p.pos += 2
		if op == "&&" && p.peek() == '&' {
			return false, "Invalid character in character class"
		}
		next, reason := p.classSetOperand()
		if reason != "" {
			return false, reason
		}
		set.Items = append(set.Items, next.item)
		if op == "&&" {
			mayStrings = mayStrings && next.strings
		}
	}
}

// classSetOperand parses a NestedClass, ClassStringDisjunction or
// ClassSetCharacter.
func (p *parser) classSetOperand() (setOperand, string) {
	if p.eof() {
		return setOperand{}, "Unterminated character class"
	}
	c := p.peek()
	if c == '[' {
		p.pos++
		nested, strs, reason := p.classSetContents()
		return setOperand{item: ClassSetItem{Kind: ClassSetNested, Nested: nested}, strings: strs}, reason
	}
	if c == '\\' {
		p.pos++
		if p.eof() {
			return setOperand{}, "\\ at end of pattern"
		}
		if p.peek() == 'q' {
			p.pos++
			return p.classStringDisjunction()
		}
		start := p.pos
		if isClass, strs, reason := p.characterClassEscape(); isClass || reason != "" {
			item := ClassSetItem{Kind: ClassSetEscape, Escape: p.src[start]}
			if item.Escape == 'p' || item.Escape == 'P' {
				item.Kind = ClassSetProperty
				item.Property = string(p.src[start+2 : p.pos-1])
			}
			return setOperand{item: item, strings: strs}, reason
		}
		v, reason := p.classSetEscape()
		return setOperand{item: ClassSetItem{Kind: ClassSetChar, Lo: v}}, reason
	}
	v, reason := p.classSetCharacter()
	return setOperand{item: ClassSetItem{Kind: ClassSetChar, Lo: v}}, reason
}

// classSetCharacter parses an unescaped ClassSetCharacter.
func (p *parser) classSetCharacter() (rune, string) {
	c := p.peek()
	if isClassSetSyntaxCharacter(c) {
		return 0, "Invalid character in character class"
	}
	if isClassSetReservedDoublePunctuator(c) && p.peekAt(1) == c {
		return 0, "Invalid set operation in character class"
	}
	p.pos++
	return c, ""
}

// classSetEscape parses the escaped ClassSetCharacter forms after '\'.
func (p *parser) classSetEscape() (rune, string) {
	c := p.peek()
	if c == 'b' {
		p.pos++
		return '\b', ""
	}
	if isClassSetReservedPunctuator(c) {
		p.pos++
		return c, ""
	}
	v, _, reason := p.characterEscape()
	return v, reason
}

// classStringDisjunction parses `{ ClassString ( | ClassString )* }` after \q.
func (p *parser) classStringDisjunction() (setOperand, string) {
	if !p.eat('{') {
		return setOperand{}, "Invalid escape"
	}
	item := ClassSetItem{Kind: ClassSetStrings}
	strs := false
	var cur []rune
	for {
		if p.eof() {
			return setOperand{}, "Unterminated character class"
		}
		switch p.peek() {
		case '}', '|':
			if len(cur) != 1 {
				strs = true
			}
			item.Strings = append(item.Strings, cur)
			cur = nil
			if p.src[p.pos] == '}' {
				p.pos++
				return setOperand{item: item, strings: strs}, ""
			}
			p.pos++
			continue
		case '\\':
			p.pos++
			if p.eof() {
				return setOperand{}, "\\ at end of pattern"
			}
			v, reason := p.classSetEscape()
			if reason != "" {
				return setOperand{}, reason
			}
			cur = append(cur, v)
		default:
			v, reason := p.classSetCharacter()
			if reason != "" {
				return setOperand{}, reason
			}
			cur = append(cur, v)
		}
	}
}

package vm

import (
	"fmt"
	"regexp"
	"strings"
	"unsafe"

	"github.com/dlclark/regexp2"
)

// cachedCompiledRegex holds the compiled regex engines that can be shared
// across multiple RegExpObject instances. The compiled engines are immutable
// so sharing is safe.
type cachedCompiledRegex struct {
	compiledRegex  *regexp.Regexp  // Go's RE2 engine (fast path)
	compiledRegex2 *regexp2.Regexp // regexp2 fallback (full ECMAScript)
	compileError   string          // Error if both engines failed
}

// RegExpObject represents a JavaScript RegExp object backed by Go's regexp package
// Uses Go's standard regexp (RE2) for simple patterns, falls back to regexp2 for
// advanced features like lookahead and backreferences.
type RegExpObject struct {
	Object                        // Embed the base Object for properties and prototype
	compiledRegex  *regexp.Regexp // Go's compiled regex engine (fast, RE2)
	compiledRegex2 *regexp2.Regexp // Fallback regex engine (slower, full ECMAScript support)
	source         string          // Original pattern string (without slashes)
	flags          string          // JavaScript flags (g, i, m, s, u, y)
	global         bool            // Cached global flag for performance
	ignoreCase     bool            // Cached ignoreCase flag
	multiline      bool            // Cached multiline flag
	dotAll         bool            // Cached dotAll flag
	lastIndex      int             // For global regex stateful matching
	Properties     *PlainObject    // Storage for user-defined properties
	compileError   string          // If non-empty, regex couldn't be compiled by either engine
}

// NewRegExp creates a new RegExp object from pattern and flags
func NewRegExp(pattern, flags string) (Value, error) {
	// ECMAScript forbids line terminators (U+2028, U+2029) in regex patterns
	if strings.ContainsRune(pattern, '\u2028') || strings.ContainsRune(pattern, '\u2029') {
		return Undefined, fmt.Errorf("Invalid regular expression: line terminator in pattern")
	}

	// ECMAScript forbids lone surrogates in regex patterns
	// In UTF-8, surrogates are encoded as: ED [A0-BF] [80-BF]
	patternBytes := []byte(pattern)
	for i := 0; i < len(patternBytes)-2; i++ {
		if patternBytes[i] == 0xED && patternBytes[i+1] >= 0xA0 && patternBytes[i+1] <= 0xBF {
			return Undefined, fmt.Errorf("Invalid regular expression: lone surrogate in pattern")
		}
	}

	// Translate JavaScript flags to Go regex pattern
	goPattern, err := translateJSFlagsToGo(pattern, flags)
	if err != nil {
		return Undefined, err
	}

	// Compile the Go regex
	compiledRegex, err := regexp.Compile(goPattern)
	if err != nil {
		return Undefined, err
	}

	// Parse individual flags
	global := strings.Contains(flags, "g")
	ignoreCase := strings.Contains(flags, "i")
	multiline := strings.Contains(flags, "m")
	dotAll := strings.Contains(flags, "s")

	regexObj := &RegExpObject{
		Object:        Object{}, // Initialize base object
		compiledRegex: compiledRegex,
		source:        pattern,
		flags:         flags,
		global:        global,
		ignoreCase:    ignoreCase,
		multiline:     multiline,
		dotAll:        dotAll,
		lastIndex:     0,
	}

	return RegExpValue(regexObj), nil
}

// compileRegexEngines compiles the regex pattern with both engines and returns a cached result.
// This is called once per unique (pattern, flags) combination.
func compileRegexEngines(pattern, flags string) *cachedCompiledRegex {
	ignoreCase := strings.Contains(flags, "i")
	multiline := strings.Contains(flags, "m")
	dotAll := strings.Contains(flags, "s")

	var compiledRegex *regexp.Regexp
	var compiledRegex2 *regexp2.Regexp
	var compileError string

	// Try Go's standard regexp (RE2) first - it's faster
	goPattern, err := translateJSFlagsToGo(pattern, flags)
	if err == nil {
		compiledRegex, _ = regexp.Compile(goPattern)
	}

	// If RE2 failed, try regexp2 (full ECMAScript support)
	if compiledRegex == nil {
		// Build regexp2 options
		opts := regexp2.RegexOptions(regexp2.ECMAScript)
		if ignoreCase {
			opts |= regexp2.RegexOptions(regexp2.IgnoreCase)
		}
		if multiline {
			opts |= regexp2.RegexOptions(regexp2.Multiline)
		}
		if dotAll {
			opts |= regexp2.RegexOptions(regexp2.Singleline) // In regexp2, Singleline makes . match \n
		}

		compiledRegex2, err = regexp2.Compile(pattern, opts)
		if err != nil {
			compileError = err.Error()
		}
	}

	return &cachedCompiledRegex{
		compiledRegex:  compiledRegex,
		compiledRegex2: compiledRegex2,
		compileError:   compileError,
	}
}

// getOrCompileRegex returns a cached compiled regex or compiles a new one.
// The compiled engines are shared across RegExpObject instances for performance.
// Uses the VM's per-instance cache instead of a global cache.
func (vm *VM) getOrCompileRegex(pattern, flags string) *cachedCompiledRegex {
	key := pattern + "\x00" + flags

	// Fast path: check if already cached
	if vm.regexCache != nil {
		if cached, ok := vm.regexCache[key]; ok {
			return cached
		}
	}

	// Slow path: compile and cache
	compiled := compileRegexEngines(pattern, flags)
	if vm.regexCache == nil {
		vm.regexCache = make(map[string]*cachedCompiledRegex)
	}
	vm.regexCache[key] = compiled
	return compiled
}

// NewRegExpDeferred creates a RegExp object, trying Go's fast RE2 engine first,
// then falling back to regexp2 for advanced features like lookahead/backreferences.
// The compiled regex engines are cached and shared across instances for performance,
// but each call returns a distinct RegExpObject (per ECMAScript spec requirement
// that each evaluation of a regex literal creates a new object).
func (vm *VM) NewRegExpDeferred(pattern, flags string) Value {
	// ECMAScript forbids line terminators (U+2028, U+2029) in regex literals
	if strings.ContainsRune(pattern, '\u2028') || strings.ContainsRune(pattern, '\u2029') {
		regexObj := &RegExpObject{
			Object:       Object{},
			source:       pattern,
			flags:        flags,
			compileError: "Invalid regular expression: line terminator in pattern",
		}
		return RegExpValue(regexObj)
	}

	// ECMAScript forbids lone surrogates (unpaired high/low surrogates) in regex patterns
	// In UTF-8, surrogates are encoded as: ED [A0-BF] [80-BF]
	// High surrogates (U+D800-U+DBFF): ED [A0-AF] [80-BF]
	// Low surrogates (U+DC00-U+DFFF): ED [B0-BF] [80-BF]
	patternBytes := []byte(pattern)
	for i := 0; i < len(patternBytes)-2; i++ {
		if patternBytes[i] == 0xED && patternBytes[i+1] >= 0xA0 && patternBytes[i+1] <= 0xBF {
			regexObj := &RegExpObject{
				Object:       Object{},
				source:       pattern,
				flags:        flags,
				compileError: "Invalid regular expression: lone surrogate in pattern",
			}
			return RegExpValue(regexObj)
		}
	}

	// Get cached compiled engines (or compile if first time)
	cached := vm.getOrCompileRegex(pattern, flags)

	// Parse individual flags for this instance
	global := strings.Contains(flags, "g")
	ignoreCase := strings.Contains(flags, "i")
	multiline := strings.Contains(flags, "m")
	dotAll := strings.Contains(flags, "s")

	// Create a NEW RegExpObject wrapper each time (per ECMAScript spec)
	// but share the compiled engines from cache for performance
	regexObj := &RegExpObject{
		Object:         Object{},
		compiledRegex:  cached.compiledRegex,  // Shared from cache
		compiledRegex2: cached.compiledRegex2, // Shared from cache
		source:         pattern,
		flags:          flags,
		global:         global,
		ignoreCase:     ignoreCase,
		multiline:      multiline,
		dotAll:         dotAll,
		lastIndex:      0,                   // Fresh per instance
		compileError:   cached.compileError, // Shared from cache
	}

	return RegExpValue(regexObj)
}

// HasCompileError returns true if this regex has a deferred compilation error
func (r *RegExpObject) HasCompileError() bool {
	return r.compileError != ""
}

// GetCompileError returns the compilation error message, or empty string if no error
func (r *RegExpObject) GetCompileError() string {
	return r.compileError
}

// UsesRegexp2 returns true if this regex uses the regexp2 fallback engine
func (r *RegExpObject) UsesRegexp2() bool {
	return r.compiledRegex == nil && r.compiledRegex2 != nil
}

// MatchString returns true if the pattern matches the string
func (r *RegExpObject) MatchString(s string) bool {
	if r.compiledRegex != nil {
		return r.compiledRegex.MatchString(s)
	}
	if r.compiledRegex2 != nil {
		match, _ := r.compiledRegex2.MatchString(s)
		return match
	}
	return false
}

// FindStringSubmatch returns the leftmost match and any captured submatches
func (r *RegExpObject) FindStringSubmatch(s string) []string {
	if r.compiledRegex != nil {
		return r.compiledRegex.FindStringSubmatch(s)
	}
	if r.compiledRegex2 != nil {
		match, err := r.compiledRegex2.FindStringMatch(s)
		if err != nil || match == nil {
			return nil
		}
		groups := match.Groups()
		result := make([]string, len(groups))
		for i, g := range groups {
			result[i] = g.String()
		}
		return result
	}
	return nil
}

// FindStringSubmatchIndex returns the index pairs for the leftmost match
func (r *RegExpObject) FindStringSubmatchIndex(s string) []int {
	if r.compiledRegex != nil {
		return r.compiledRegex.FindStringSubmatchIndex(s)
	}
	if r.compiledRegex2 != nil {
		match, err := r.compiledRegex2.FindStringMatch(s)
		if err != nil || match == nil {
			return nil
		}
		groups := match.Groups()
		result := make([]int, len(groups)*2)
		for i, g := range groups {
			if g.Length > 0 {
				result[i*2] = g.Index
				result[i*2+1] = g.Index + g.Length
			} else {
				result[i*2] = -1
				result[i*2+1] = -1
			}
		}
		return result
	}
	return nil
}

// FindAllStringSubmatchIndex returns all matches with their indices
func (r *RegExpObject) FindAllStringSubmatchIndex(s string, n int) [][]int {
	if r.compiledRegex != nil {
		return r.compiledRegex.FindAllStringSubmatchIndex(s, n)
	}
	if r.compiledRegex2 != nil {
		var results [][]int
		match, err := r.compiledRegex2.FindStringMatch(s)
		for err == nil && match != nil && (n < 0 || len(results) < n) {
			groups := match.Groups()
			indices := make([]int, len(groups)*2)
			for i, g := range groups {
				if g.Length > 0 {
					indices[i*2] = g.Index
					indices[i*2+1] = g.Index + g.Length
				} else {
					indices[i*2] = -1
					indices[i*2+1] = -1
				}
			}
			results = append(results, indices)
			match, err = r.compiledRegex2.FindNextMatch(match)
		}
		return results
	}
	return nil
}

// FindAllString returns all successive matches
func (r *RegExpObject) FindAllString(s string, n int) []string {
	if r.compiledRegex != nil {
		return r.compiledRegex.FindAllString(s, n)
	}
	if r.compiledRegex2 != nil {
		var results []string
		match, err := r.compiledRegex2.FindStringMatch(s)
		for err == nil && match != nil && (n < 0 || len(results) < n) {
			results = append(results, match.String())
			match, err = r.compiledRegex2.FindNextMatch(match)
		}
		return results
	}
	return nil
}

// FindStringIndex returns the index of the leftmost match
func (r *RegExpObject) FindStringIndex(s string) []int {
	if r.compiledRegex != nil {
		return r.compiledRegex.FindStringIndex(s)
	}
	if r.compiledRegex2 != nil {
		match, err := r.compiledRegex2.FindStringMatch(s)
		if err != nil || match == nil {
			return nil
		}
		return []int{match.Index, match.Index + match.Length}
	}
	return nil
}

// ReplaceAllString replaces all matches with the replacement string
func (r *RegExpObject) ReplaceAllString(src, repl string) string {
	if r.compiledRegex != nil {
		return r.compiledRegex.ReplaceAllString(src, repl)
	}
	if r.compiledRegex2 != nil {
		result, _ := r.compiledRegex2.Replace(src, repl, -1, -1)
		return result
	}
	return src
}

// ReplaceAllStringFunc replaces all matches using a function
func (r *RegExpObject) ReplaceAllStringFunc(src string, repl func(string) string) string {
	if r.compiledRegex != nil {
		return r.compiledRegex.ReplaceAllStringFunc(src, repl)
	}
	if r.compiledRegex2 != nil {
		// regexp2 doesn't have a direct equivalent, so we implement it manually
		var result strings.Builder
		lastEnd := 0
		match, err := r.compiledRegex2.FindStringMatch(src)
		for err == nil && match != nil {
			result.WriteString(src[lastEnd:match.Index])
			result.WriteString(repl(match.String()))
			lastEnd = match.Index + match.Length
			match, err = r.compiledRegex2.FindNextMatch(match)
		}
		result.WriteString(src[lastEnd:])
		return result.String()
	}
	return src
}

// Split splits the string by the regex pattern
func (r *RegExpObject) Split(s string, n int) []string {
	if r.compiledRegex != nil {
		return r.compiledRegex.Split(s, n)
	}
	if r.compiledRegex2 != nil {
		// Implement split for regexp2
		if n == 0 {
			return nil
		}
		var results []string
		lastEnd := 0
		match, err := r.compiledRegex2.FindStringMatch(s)
		for err == nil && match != nil && (n < 0 || len(results) < n-1) {
			results = append(results, s[lastEnd:match.Index])
			lastEnd = match.Index + match.Length
			match, err = r.compiledRegex2.FindNextMatch(match)
		}
		results = append(results, s[lastEnd:])
		return results
	}
	return []string{s}
}

// preprocessUnicodeEscapes converts JavaScript \uXXXX and \xXX escapes to actual Unicode characters
// This is needed because Go's regexp doesn't support \u escapes
func preprocessUnicodeEscapes(pattern string) string {
	result := strings.Builder{}
	i := 0
	for i < len(pattern) {
		if i+1 < len(pattern) && pattern[i] == '\\' {
			switch pattern[i+1] {
			case 'u':
				// Handle \uXXXX (4 hex digits)
				if i+5 < len(pattern) {
					hex := pattern[i+2 : i+6]
					if isValidHex(hex) {
						codePoint := parseHex(hex)
						result.WriteRune(rune(codePoint))
						i += 6
						continue
					}
				}
				// Handle \u{XXXXX} (1-6 hex digits, for unicode flag)
				if i+3 < len(pattern) && pattern[i+2] == '{' {
					end := strings.Index(pattern[i+3:], "}")
					if end != -1 && end <= 6 {
						hex := pattern[i+3 : i+3+end]
						if isValidHex(hex) {
							codePoint := parseHex(hex)
							result.WriteRune(rune(codePoint))
							i += 4 + end
							continue
						}
					}
				}
			case 'x':
				// Handle \xXX (2 hex digits)
				if i+3 < len(pattern) {
					hex := pattern[i+2 : i+4]
					if isValidHex(hex) {
						codePoint := parseHex(hex)
						result.WriteRune(rune(codePoint))
						i += 4
						continue
					}
				}
			}
		}
		result.WriteByte(pattern[i])
		i++
	}
	return result.String()
}

// isValidHex checks if a string contains only valid hex digits
func isValidHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// parseHex parses a hex string to an integer
func parseHex(s string) int {
	result := 0
	for _, c := range s {
		result *= 16
		switch {
		case c >= '0' && c <= '9':
			result += int(c - '0')
		case c >= 'a' && c <= 'f':
			result += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			result += int(c-'A') + 10
		}
	}
	return result
}

// translateJSFlagsToGo converts JavaScript regex flags to Go inline flag syntax
func translateJSFlagsToGo(pattern, flags string) (string, error) {
	// Preprocess Unicode escapes (\uXXXX, \xXX) that Go's regexp doesn't support
	pattern = preprocessUnicodeEscapes(pattern)

	// Check for JavaScript features that Go's regexp doesn't support
	// Go's regexp library doesn't support numbered backreferences like \1, \2, etc.
	for i := 1; i <= 9; i++ {
		backref := fmt.Sprintf(`\%d`, i)
		if strings.Contains(pattern, backref) {
			return "", fmt.Errorf("numbered backreferences like %s are not supported (Go regexp limitation)", backref)
		}
	}

	goPattern := pattern

	// Apply flags as Go inline flags (prepended to pattern)
	var flagPrefixes []string

	if strings.Contains(flags, "i") {
		flagPrefixes = append(flagPrefixes, "(?i)")
	}
	if strings.Contains(flags, "m") {
		flagPrefixes = append(flagPrefixes, "(?m)")
	}
	if strings.Contains(flags, "s") {
		flagPrefixes = append(flagPrefixes, "(?s)")
	}

	// Note: 'g' (global) and 'y' (sticky) are handled at the JavaScript level,
	// not in Go's regex engine. 'u' (unicode) is default in Go.

	// Prepend all flag prefixes to the pattern
	if len(flagPrefixes) > 0 {
		goPattern = strings.Join(flagPrefixes, "") + goPattern
	}

	return goPattern, nil
}

// AsRegExp extracts a RegExpObject from a Value
func AsRegExp(v Value) *RegExpObject {
	return (*RegExpObject)(v.obj)
}

// RegExpValue creates a Value from a RegExpObject
func RegExpValue(r *RegExpObject) Value {
	return Value{
		typ: TypeRegExp,
		obj: unsafe.Pointer(r),
	}
}

// IsRegExp checks if a Value is a RegExp
func (v Value) IsRegExp() bool {
	return v.typ == TypeRegExp
}

// AsRegExpObject safely converts a Value to a RegExpObject, returns nil if not a regex
func (v Value) AsRegExpObject() *RegExpObject {
	if v.typ != TypeRegExp {
		return nil
	}
	return AsRegExp(v)
}

// Getter methods for RegExpObject
func (r *RegExpObject) GetSource() string {
	return r.source
}

func (r *RegExpObject) GetFlags() string {
	return r.flags
}

func (r *RegExpObject) GetCompiledRegex() *regexp.Regexp {
	return r.compiledRegex
}

func (r *RegExpObject) IsGlobal() bool {
	return r.global
}

func (r *RegExpObject) IsIgnoreCase() bool {
	return r.ignoreCase
}

func (r *RegExpObject) IsMultiline() bool {
	return r.multiline
}

func (r *RegExpObject) IsDotAll() bool {
	return r.dotAll
}

func (r *RegExpObject) GetLastIndex() int {
	return r.lastIndex
}

func (r *RegExpObject) SetLastIndex(index int) {
	if index < 0 {
		index = 0
	}
	r.lastIndex = index
}
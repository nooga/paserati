package vm

import (
	"fmt"
	"regexp"
	"strings"
	"unsafe"
)

// RegExpObject represents a JavaScript RegExp object backed by Go's regexp package
type RegExpObject struct {
	Object                       // Embed the base Object for properties and prototype
	compiledRegex *regexp.Regexp // Go's compiled regex engine
	source        string         // Original pattern string (without slashes)
	flags         string         // JavaScript flags (g, i, m, s, u, y)
	global        bool           // Cached global flag for performance
	ignoreCase    bool           // Cached ignoreCase flag
	multiline     bool           // Cached multiline flag
	dotAll        bool           // Cached dotAll flag
	lastIndex     int            // For global regex stateful matching
	Properties    *PlainObject   // Storage for user-defined properties
}

// NewRegExp creates a new RegExp object from pattern and flags
func NewRegExp(pattern, flags string) (Value, error) {
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
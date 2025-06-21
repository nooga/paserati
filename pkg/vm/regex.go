package vm

import (
	"regexp"
	"strings"
	"unsafe"
)

// RegExpObject represents a JavaScript RegExp object backed by Go's regexp package
type RegExpObject struct {
	Object                      // Embed the base Object for properties and prototype
	compiledRegex *regexp.Regexp // Go's compiled regex engine
	source        string        // Original pattern string (without slashes)
	flags         string        // JavaScript flags (g, i, m, s, u, y)
	global        bool          // Cached global flag for performance
	ignoreCase    bool          // Cached ignoreCase flag
	multiline     bool          // Cached multiline flag
	dotAll        bool          // Cached dotAll flag
	lastIndex     int           // For global regex stateful matching
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

// translateJSFlagsToGo converts JavaScript regex flags to Go inline flag syntax
func translateJSFlagsToGo(pattern, flags string) (string, error) {
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
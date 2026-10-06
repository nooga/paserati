package main

import (
	"regexp"
	"sort"
	"strings"
)

// This file mirrors the parts of TypeScript's test harness
// (src/harness/harnessIO.ts: extractCompilerSettings / makeUnitsFromTest and
// getFileBasedTestConfigurations in v6.0.x) that decide what source the
// compiler actually sees and which option combinations a test is run under.

// directiveRegex is TypeScript's `optionRegex`, applied to a single line. It is
// NOT trimmed first: an indented `// @x: y` is ordinary code in TypeScript.
var directiveRegex = regexp.MustCompile(`^/{2}\s*@(\w+)\s*:\s*([^\r\n]*)`)

// linkRegex is TypeScript's `linkRegex` (`// @link: a -> b`). makeUnitsFromTest
// consumes such lines as symlink declarations, so they are not part of the
// source either.
var linkRegex = regexp.MustCompile(`^/{2}\s*@link\s*:\s*([^\r\n]*)\s*->\s*([^\r\n]*)`)

// newlineRegex is the harness's `splitContentByNewlines` separator. A lone \r
// is deliberately not a separator.
var newlineRegex = regexp.MustCompile(`\r?\n`)

// Options is the set of compiler options a test is configured with, keyed by
// lower-cased option name. Values are trimmed, original-case strings.
type Options map[string]string

// StripResult is the outcome of running makeUnitsFromTest over a single-file
// test.
type StripResult struct {
	Source       string  // what the compiler sees; baseline line numbers refer to this
	Options      Options // every `// @key: value` directive (lower-cased keys, last one wins)
	HasFilename  bool    // a `// @filename:` directive is present (multi-file test)
	StrippedLine int     // number of directive/link lines removed
}

// stripDirectives removes every directive line from source exactly like
// TestCaseParser.makeUnitsFromTest does for a test without `@filename`:
//
//   - the file is split on \r?\n and re-joined with \n (so CRLF is normalised);
//   - lines matching the directive regex (or the symlink regex) are dropped;
//   - the first line of content is appended without a leading newline, and
//     because the harness only inserts a "\n" separator when the accumulated
//     content is non-empty, blank lines that precede the first non-blank line
//     of content vanish as well. This is a quirk of the harness, but baseline
//     line numbers depend on it.
func stripDirectives(source string) StripResult {
	res := StripResult{Options: Options{}}
	lines := newlineRegex.Split(source, -1)
	var b strings.Builder
	nonEmpty := false
	for _, line := range lines {
		if linkRegex.MatchString(line) {
			res.StrippedLine++
			continue
		}
		if m := directiveRegex.FindStringSubmatch(line); m != nil {
			key := strings.ToLower(m[1])
			res.Options[key] = strings.TrimSpace(m[2])
			if key == "filename" {
				res.HasFilename = true
			}
			res.StrippedLine++
			continue
		}
		if nonEmpty {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if b.Len() > 0 {
			nonEmpty = true
		}
	}
	res.Source = b.String()
	return res
}

// parseDirectives extracts the `// @key: value` directives (lower-cased keys).
// It is the single definition of "is this line a directive" shared by
// multi-file detection and option handling.
func parseDirectives(source string) Options {
	opts := Options{}
	for _, line := range newlineRegex.Split(source, -1) {
		if linkRegex.MatchString(line) {
			continue
		}
		if m := directiveRegex.FindStringSubmatch(line); m != nil {
			opts[strings.ToLower(m[1])] = strings.TrimSpace(m[2])
		}
	}
	return opts
}

// isMultiFile reports whether the test declares `// @filename:` units
// (case-insensitively, like the harness: `@Filename`, `@fileName`, ...).
func isMultiFile(source string) bool {
	_, ok := parseDirectives(source)["filename"]
	return ok
}

// filterSingleFileTests splits files into single-file tests and multi-file
// tests, using the same directive definition as parseDirectives.
func filterSingleFileTests(readFile func(string) ([]byte, error), files []string) (single, multi []string) {
	for _, f := range files {
		content, err := readFile(f)
		if err != nil {
			continue
		}
		if isMultiFile(string(content)) {
			multi = append(multi, f)
		} else {
			single = append(single, f)
		}
	}
	return single, multi
}

// ---------------------------------------------------------------------------
// Option accessors

// singleValue returns the option's value when it carries exactly one
// non-empty comma-separated entry (so `true,` is "true"), and ok=false when it
// is missing, empty or genuinely multi-valued.
func (o Options) singleValue(key string) (string, bool) {
	v, present := o[key]
	if !present {
		return "", false
	}
	var parts []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) != 1 {
		return "", false
	}
	return parts[0], true
}

// boolOpt returns the value of a boolean option and whether it was explicitly
// (and unambiguously) set.
func (o Options) boolOpt(key string) (val, set bool) {
	v, ok := o.singleValue(key)
	if !ok {
		return false, false
	}
	switch strings.ToLower(v) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

// strictFamily resolves an option of the strict family: an explicit directive
// wins, then `strict`, then the TypeScript 6.0 default (on).
func (o Options) strictFamily(key string) bool {
	if v, ok := o.boolOpt(key); ok {
		return v
	}
	if v, ok := o.boolOpt("strict"); ok {
		return v
	}
	return true
}

// ---------------------------------------------------------------------------
// Variants

// Variant is one concrete option combination a test is run under. Keys and
// values are lower-cased, exactly as they appear in a parameterised baseline
// name such as `foo(strict=false,target=es5).errors.txt`.
type Variant map[string]string

// Suffix renders the variant like TypeScript names its baselines: keys sorted,
// `(k=v,k=v)`. The empty variant renders as "".
func (v Variant) Suffix() string {
	if len(v) == 0 {
		return ""
	}
	return "(" + v.canonical() + ")"
}

func (v Variant) canonical() string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + v[k]
	}
	return strings.Join(parts, ",")
}

// matchKey is canonical() with enum aliases folded (es6 == es2015) so a variant
// finds its baseline whichever spelling the test or the baseline used.
func (v Variant) matchKey() string {
	w := Variant{}
	for k, val := range v {
		w[k] = normalizeOptionValue(k, val)
	}
	return w.canonical()
}

func normalizeOptionValue(key, val string) string {
	if (key == "target" || key == "module") && val == "es6" {
		return "es2015"
	}
	return val
}

type enumEntry struct {
	name string
	id   int
}

// enumOptions are the enum-typed compiler options a directive may vary by.
// The ids exist only so aliases (es6/es2015) dedupe like TypeScript's
// value-based dedupe; the order is the order `*` expands in.
var enumOptions = map[string][]enumEntry{
	"target": {
		{"es3", 0}, {"es5", 1}, {"es6", 2}, {"es2015", 2}, {"es2016", 3}, {"es2017", 4},
		{"es2018", 5}, {"es2019", 6}, {"es2020", 7}, {"es2021", 8}, {"es2022", 9},
		{"es2023", 10}, {"es2024", 11}, {"es2025", 12}, {"esnext", 99},
	},
	"module": {
		{"none", 0}, {"commonjs", 1}, {"amd", 2}, {"umd", 3}, {"system", 4},
		{"es6", 5}, {"es2015", 5}, {"es2020", 6}, {"es2022", 7}, {"esnext", 99},
		{"node16", 100}, {"node18", 101}, {"node20", 102}, {"nodenext", 199}, {"preserve", 200},
	},
	"moduleresolution": {
		{"classic", 1}, {"node10", 2}, {"node", 2}, {"node16", 3}, {"nodenext", 99}, {"bundler", 100},
	},
	"moduledetection": {{"legacy", 1}, {"auto", 2}, {"force", 3}},
	"jsx":             {{"preserve", 1}, {"react", 2}, {"react-native", 3}, {"react-jsx", 4}, {"react-jsxdev", 5}},
	"newline":         {{"crlf", 0}, {"lf", 1}},
}

// nonVaryOptions are directives that carry lists/strings/paths (or harness
// settings) and so never generate variants even when they contain commas.
var nonVaryOptions = map[string]bool{
	"lib": true, "types": true, "typeroots": true, "rootdirs": true, "paths": true,
	"filename": true, "link": true, "outfile": true, "outdir": true, "rootdir": true,
	"baseurl": true, "jsxfactory": true, "jsxfragmentfactory": true, "jsximportsource": true,
	"reactnamespace": true, "declarationdir": true, "maproot": true, "sourceroot": true,
	"tsbuildinfofile": true, "customconditions": true, "modulesuffixes": true,
	"currentdirectory": true, "symlink": true,
}

func isBoolToken(s string) bool {
	switch s {
	case "true", "false", "*":
		return true
	}
	return false
}

// splitVaryBy mirrors splitVaryBySettingValue. It returns the ordered, deduped
// entries a directive expands to, or nil when the directive does not vary.
func splitVaryBy(key, text string) []string {
	if text == "" || nonVaryOptions[key] {
		return nil
	}
	star := false
	var includes, excludes []string
	for _, s := range strings.Split(text, ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		switch {
		case s == "":
		case s == "*":
			star = true
		case strings.HasPrefix(s, "-") || strings.HasPrefix(s, "!"):
			excludes = append(excludes, s[1:])
		default:
			includes = append(includes, s)
		}
	}
	if len(includes) <= 1 && !star && len(excludes) == 0 {
		return nil
	}

	// Which option kind? Enum options vary on their known entries; anything else
	// varies only when every token is boolean-looking (boolean options).
	enum, isEnum := enumOptions[key]
	var values []enumEntry
	if isEnum {
		values = enum
	} else {
		for _, t := range append(append([]string{}, includes...), excludes...) {
			if !isBoolToken(t) {
				return nil
			}
		}
		values = []enumEntry{{"true", 1}, {"false", 0}}
	}
	idOf := func(name string) (int, bool) {
		for _, e := range values {
			if e.name == name {
				return e.id, true
			}
		}
		return 0, false
	}

	type variation struct {
		key   string
		id    int
		hasID bool
	}
	var vars []variation
	find := func(key string, id int, hasID bool) int {
		for i, v := range vars {
			if v.key == key || (hasID && v.hasID && v.id == id) {
				return i
			}
		}
		return -1
	}
	for _, inc := range includes {
		id, ok := idOf(inc)
		if find(inc, id, ok) < 0 {
			vars = append(vars, variation{inc, id, ok})
		}
	}
	if star {
		for _, e := range values {
			if find(e.name, e.id, true) < 0 {
				vars = append(vars, variation{e.name, e.id, true})
			}
		}
	}
	for _, ex := range excludes {
		id, ok := idOf(ex)
		for {
			i := find(ex, id, ok)
			if i < 0 {
				break
			}
			vars = append(vars[:i], vars[i+1:]...)
		}
	}
	if len(vars) == 0 {
		return nil
	}
	out := make([]string, len(vars))
	for i, v := range vars {
		out[i] = v.key
	}
	return out
}

// expandVariants returns every option combination the test runs under, in a
// deterministic order. A test with no varying directive yields one empty
// variant. TypeScript caps the product at 25; beyond that we also stop (and
// report it) rather than explode.
func expandVariants(opts Options) (variants []Variant, tooMany bool) {
	var keys []string
	for k := range opts {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type axis struct {
		key     string
		entries []string
	}
	var axes []axis
	count := 1
	for _, k := range keys {
		entries := splitVaryBy(k, opts[k])
		if entries == nil {
			continue
		}
		count *= len(entries)
		if count > 25 {
			return []Variant{{}}, true
		}
		axes = append(axes, axis{k, entries})
	}
	variants = []Variant{{}}
	for _, ax := range axes {
		var next []Variant
		for _, base := range variants {
			for _, e := range ax.entries {
				v := Variant{}
				for k, val := range base {
					v[k] = val
				}
				v[ax.key] = e
				next = append(next, v)
			}
		}
		variants = next
	}
	return variants, false
}

// effectiveOptions applies a variant's single values over the raw directives.
func effectiveOptions(raw Options, v Variant) Options {
	eff := Options{}
	for k, val := range raw {
		eff[k] = val
	}
	for k, val := range v {
		eff[k] = val
	}
	return eff
}

// ---------------------------------------------------------------------------
// Option support audit

// supportedOptions are the directives createTscPaserati actually honours.
var supportedOptions = map[string]bool{
	"strict": true, "strictnullchecks": true, "strictpropertyinitialization": true,
	"alwaysstrict": true, "noimplicitoverride": true, "allowunreachablecode": true,
}

// unmodeledOptions are directives that plausibly change TypeScript's
// diagnostics and that Paserati does not model. Directives that only affect
// emit (sourceMap, outFile, ...) are not listed.
var unmodeledOptions = map[string]bool{
	"noimplicitany": true, "noimplicitthis": true, "noimplicitreturns": true,
	"strictfunctiontypes": true, "strictbindcallapply": true, "strictbuiltiniteratorreturn": true,
	"exactoptionalpropertytypes": true, "nouncheckedindexedaccess": true,
	"useunknownincatchvariables": true, "nopropertyaccessfromindexsignature": true,
	"nolib": true, "lib": true, "target": true, "module": true, "moduleresolution": true,
	"moduledetection": true, "jsx": true, "jsxfactory": true, "jsxfragmentfactory": true,
	"jsximportsource": true, "reactnamespace": true,
	"experimentaldecorators": true, "emitdecoratormetadata": true,
	"allowjs": true, "checkjs": true, "maxnodemodulejsdepth": true,
	"declaration": true, "isolateddeclarations": true, "isolatedmodules": true,
	"verbatimmodulesyntax": true, "nounusedlocals": true, "nounusedparameters": true,
	"nofallthroughcasesinswitch": true, "allowsyntheticdefaultimports": true,
	"esmoduleinterop": true, "usedefineforclassfields": true, "downleveliteration": true,
	"importhelpers": true, "noemithelpers": true, "resolvejsonmodule": true,
	"preserveconstenums": true, "noresolve": true, "types": true, "typeroots": true,
	"paths": true, "baseurl": true, "rootdirs": true, "allowarbitraryextensions": true,
	"allowimportingtsextensions": true, "nouncheckedsideeffectimports": true,
	"erasablesyntaxonly": true, "allowumdglobalaccess": true, "libreplacement": true,
	"noerrortruncation": true, "customconditions": true, "resolvepackagejsonexports": true,
	"resolvepackagejsonimports": true, "suppressexcesspropertyerrors": true,
	"suppressimplicitanyindexerrors": true, "allowunusedlabels": true,
	"stripinternal": true, "keyofstringsonly": true, "noimplicitusestrict": true,
	"importsnotusedasvalues": true, "preservevalueimports": true, "rewriterelativeimportextensions": true,
}

// unsupportedOptionsFor lists the directives (as `key=value`, sorted) in effect
// for this variant that Paserati does not model and that can change TS's
// diagnostics. It also tags `strict=false` as `strict(partial)` because only
// part of the strict family can be switched off.
func unsupportedOptionsFor(eff Options) []string {
	var out []string
	for k, v := range eff {
		if supportedOptions[k] || !unmodeledOptions[k] {
			continue
		}
		out = append(out, k+"="+strings.ToLower(v))
	}
	if sv, ok := eff.boolOpt("strict"); ok && !sv {
		out = append(out, "strict(partial)")
	}
	sort.Strings(out)
	return out
}

// optionKey is the part of an unsupported-option entry before `=` / `(`.
func optionKey(entry string) string {
	if i := strings.IndexAny(entry, "=("); i >= 0 {
		return entry[:i]
	}
	return entry
}

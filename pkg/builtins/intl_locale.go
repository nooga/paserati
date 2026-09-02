package builtins

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"golang.org/x/text/language"

	"github.com/nooga/paserati/pkg/vm"
)

// ECMA-402 locale plumbing shared by the Intl constructors: language-tag
// structural validation (UTS #35 unicode_locale_id), canonicalization,
// CanonicalizeLocaleList, the default locale, and the lookup matcher.
//
// Paserati ships no CLDR locale data - the only Intl consumer so far is
// Intl.Segmenter (#210), whose default-locale segmentation is
// locale-independent (UAX #29). So "available locale" here means "a
// structurally valid tag whose language subtag is a known ISO 639 code"
// (x/text's tables stand in for the language list), and canonicalization
// covers casing, subtag ordering and the handful of deprecated ISO 639-1
// codes below rather than the full CLDR alias data. Intl.getCanonicalLocales
// is therefore best-effort where CLDR aliases are concerned.

// ---- UTS #35 unicode_locale_id grammar ----

func intlIsAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func intlIsDigit(c byte) bool { return c >= '0' && c <= '9' }
func intlIsAlnum(c byte) bool { return intlIsAlpha(c) || intlIsDigit(c) }

func intlAll(s string, pred func(byte) bool) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !pred(s[i]) {
			return false
		}
	}
	return true
}

// unicode_language_subtag = alpha{2,3} | alpha{5,8}
func intlIsLanguageSubtag(s string) bool {
	n := len(s)
	return (n == 2 || n == 3 || (n >= 5 && n <= 8)) && intlAll(s, intlIsAlpha)
}

// unicode_script_subtag = alpha{4}
func intlIsScriptSubtag(s string) bool { return len(s) == 4 && intlAll(s, intlIsAlpha) }

// unicode_region_subtag = alpha{2} | digit{3}
func intlIsRegionSubtag(s string) bool {
	return (len(s) == 2 && intlAll(s, intlIsAlpha)) || (len(s) == 3 && intlAll(s, intlIsDigit))
}

// unicode_variant_subtag = alphanum{5,8} | digit alphanum{3}
func intlIsVariantSubtag(s string) bool {
	n := len(s)
	return (n >= 5 && n <= 8 && intlAll(s, intlIsAlnum)) ||
		(n == 4 && intlIsDigit(s[0]) && intlAll(s, intlIsAlnum))
}

func intlIsAlnumRange(s string, lo, hi int) bool {
	return len(s) >= lo && len(s) <= hi && intlAll(s, intlIsAlnum)
}

// intlParseLanguageID consumes a unicode_language_id starting at parts[i]
// (language, optional script, optional region, variants) and returns the
// index just past it. Fails on a missing/ill-formed language subtag or a
// duplicate variant (ECMA-402 IsStructurallyValidLanguageTag step 4).
func intlParseLanguageID(parts []string, i int) (int, bool) {
	if i >= len(parts) || !intlIsLanguageSubtag(parts[i]) {
		return i, false
	}
	i++
	if i < len(parts) && intlIsScriptSubtag(parts[i]) {
		i++
	}
	if i < len(parts) && intlIsRegionSubtag(parts[i]) {
		i++
	}
	var seen []string
	for i < len(parts) && intlIsVariantSubtag(parts[i]) {
		v := strings.ToLower(parts[i])
		for _, s := range seen {
			if s == v {
				return i, false
			}
		}
		seen = append(seen, v)
		i++
	}
	return i, true
}

// intlIsStructurallyValidLanguageTag implements ECMA-402
// IsStructurallyValidLanguageTag: the tag matches the UTS #35
// unicode_locale_id production (BCP 47 syntax, hyphen separators only, no
// grandfathered or extlang forms), has no duplicate variant subtags and no
// duplicate extension singletons.
func intlIsStructurallyValidLanguageTag(tag string) bool {
	if tag == "" {
		return false
	}
	for i := 0; i < len(tag); i++ {
		if c := tag[i]; !intlIsAlnum(c) && c != '-' {
			return false
		}
	}
	parts := strings.Split(tag, "-")
	for _, p := range parts {
		if p == "" { // leading/trailing/double separator
			return false
		}
	}
	i, ok := intlParseLanguageID(parts, 0)
	if !ok {
		return false
	}
	var seen [128]bool
	for i < len(parts) {
		p := parts[i]
		if len(p) != 1 {
			return false
		}
		s := p[0] | 0x20 // fold letters to lowercase; digits are unaffected
		if seen[s] {
			return false
		}
		seen[s] = true
		i++
		switch s {
		case 'x':
			// pu_extensions = sep [xX] (sep alphanum{1,8})+ ; always last
			if i >= len(parts) {
				return false
			}
			for ; i < len(parts); i++ {
				if !intlIsAlnumRange(parts[i], 1, 8) {
					return false
				}
			}
			return true
		case 'u':
			// unicode_locale_extensions = [uU] ((sep keyword)+ | (sep attribute)+ (sep keyword)*)
			// attribute = alphanum{3,8}; keyword = key (sep type)?; key = alphanum alpha; type = alphanum{3,8}+
			start := i
			for i < len(parts) && intlIsAlnumRange(parts[i], 3, 8) {
				i++
			}
			for i < len(parts) && len(parts[i]) == 2 && intlIsAlnum(parts[i][0]) && intlIsAlpha(parts[i][1]) {
				i++
				for i < len(parts) && intlIsAlnumRange(parts[i], 3, 8) {
					i++
				}
			}
			if i == start {
				return false
			}
		case 't':
			// transformed_extensions = [tT] ((sep tlang (sep tfield)*) | (sep tfield)+)
			// tfield = tkey tvalue; tkey = alpha digit; tvalue = (sep alphanum{3,8})+
			start := i
			if i < len(parts) && intlIsLanguageSubtag(parts[i]) {
				n, ok := intlParseLanguageID(parts, i)
				if !ok {
					return false
				}
				i = n
			}
			for i < len(parts) && len(parts[i]) == 2 && intlIsAlpha(parts[i][0]) && intlIsDigit(parts[i][1]) {
				i++
				vs := i
				for i < len(parts) && intlIsAlnumRange(parts[i], 3, 8) {
					i++
				}
				if i == vs {
					return false
				}
			}
			if i == start {
				return false
			}
		default:
			// other_extensions = [alphanum-[tTuUxX]] (sep alphanum{2,8})+
			start := i
			for i < len(parts) && intlIsAlnumRange(parts[i], 2, 8) {
				i++
			}
			if i == start {
				return false
			}
		}
	}
	return true
}

// ---- Canonicalization ----

// Deprecated ISO 639-1 codes and their CLDR replacements - the aliases real
// code and the ECMA-402 tests most commonly exercise. Not the full CLDR
// languageAlias table (see the file comment).
var intlLanguageAliases = map[string]string{
	"iw":  "he",
	"in":  "id",
	"ji":  "yi",
	"jw":  "jv",
	"mo":  "ro",
	"tl":  "fil",
	"sh":  "sr-Latn",
	"cmn": "zh",
}

// Regular grandfathered BCP 47 tags that are structurally valid unicode_locale_ids
// (language + variant) and have a CLDR replacement.
var intlGrandfatheredAliases = map[string]string{
	"art-lojban":  "jbo",
	"cel-gaulish": "xtg",
	"zh-guoyu":    "zh",
	"zh-hakka":    "hak",
	"zh-xiang":    "hsn",
}

func intlTitleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// intlCanonicalizeUnicodeLocaleID implements the casing/ordering part of UTS
// #35's canonical Unicode locale identifier: lowercase language, titlecase
// script, uppercase region, variants sorted, extensions ordered by
// singleton with private use last, -u- attributes/keywords sorted and "true"
// types dropped, -t- fields sorted by key, plus the language aliases above.
// The tag must already be structurally valid.
func intlCanonicalizeUnicodeLocaleID(tag string) string {
	parts := strings.Split(strings.ToLower(tag), "-")
	n := len(parts)
	i := 0

	lang := parts[i]
	i++
	script, region := "", ""
	if i < n && intlIsScriptSubtag(parts[i]) {
		script = intlTitleCase(parts[i])
		i++
	}
	if i < n && intlIsRegionSubtag(parts[i]) {
		region = strings.ToUpper(parts[i])
		i++
	}
	var variants []string
	for i < n && intlIsVariantSubtag(parts[i]) {
		variants = append(variants, parts[i])
		i++
	}
	sort.Strings(variants)

	// Regular grandfathered tags (language + variant) that CLDR maps to a
	// plain language subtag, e.g. art-lojban -> jbo.
	if len(variants) == 1 {
		if alias, ok := intlGrandfatheredAliases[lang+"-"+variants[0]]; ok {
			lang = alias
			variants = nil
		}
	}
	// ISO 639-2/3 codes whose language has an ISO 639-1 code canonicalize to
	// the two-letter form (heb -> he, ces -> cs); x/text's tables know these.
	if len(lang) == 3 {
		if base, err := language.ParseBase(lang); err == nil {
			if short := base.String(); len(short) == 2 {
				lang = short
			}
		}
	}
	if alias, ok := intlLanguageAliases[lang]; ok {
		ap := strings.Split(alias, "-")
		lang = ap[0]
		if len(ap) > 1 && script == "" {
			script = ap[1]
		}
	}

	var sb strings.Builder
	sb.WriteString(lang)
	if script != "" {
		sb.WriteByte('-')
		sb.WriteString(script)
	}
	if region != "" {
		sb.WriteByte('-')
		sb.WriteString(region)
	}
	for _, v := range variants {
		sb.WriteByte('-')
		sb.WriteString(v)
	}

	type extension struct {
		singleton byte
		body      []string
	}
	var exts []extension
	var privateUse []string
	for i < n {
		s := parts[i][0]
		i++
		if s == 'x' {
			privateUse = parts[i:]
			break
		}
		start := i
		for i < n && len(parts[i]) != 1 {
			i++
		}
		exts = append(exts, extension{singleton: s, body: parts[start:i]})
	}
	sort.SliceStable(exts, func(a, b int) bool { return exts[a].singleton < exts[b].singleton })
	for _, e := range exts {
		body := e.body
		switch e.singleton {
		case 'u':
			body = intlCanonicalizeUnicodeExtension(body)
		case 't':
			body = intlCanonicalizeTransformedExtension(body)
		}
		sb.WriteByte('-')
		sb.WriteByte(e.singleton)
		for _, b := range body {
			sb.WriteByte('-')
			sb.WriteString(b)
		}
	}
	if privateUse != nil {
		sb.WriteString("-x")
		for _, b := range privateUse {
			sb.WriteByte('-')
			sb.WriteString(b)
		}
	}
	return sb.String()
}

// intlCanonicalizeUnicodeExtension sorts and dedupes the attributes, sorts
// the keywords by key keeping the first occurrence of a duplicate key, and
// drops the type "true" (UTS #35: a keyword with type "true" is canonically
// just the key).
func intlCanonicalizeUnicodeExtension(body []string) []string {
	i := 0
	var attrs []string
	for i < len(body) && len(body[i]) != 2 {
		attrs = append(attrs, body[i])
		i++
	}
	sort.Strings(attrs)
	attrs = intlDedupeSorted(attrs)

	type keyword struct {
		key   string
		types []string
	}
	var keywords []keyword
	seenKey := map[string]bool{}
	for i < len(body) {
		kw := keyword{key: body[i]}
		i++
		for i < len(body) && len(body[i]) != 2 {
			kw.types = append(kw.types, body[i])
			i++
		}
		if seenKey[kw.key] {
			continue
		}
		seenKey[kw.key] = true
		if len(kw.types) == 1 && kw.types[0] == "true" {
			kw.types = nil
		}
		keywords = append(keywords, kw)
	}
	sort.SliceStable(keywords, func(a, b int) bool { return keywords[a].key < keywords[b].key })

	out := attrs
	for _, kw := range keywords {
		out = append(out, kw.key)
		out = append(out, kw.types...)
	}
	return out
}

// intlCanonicalizeTransformedExtension keeps the tlang (already lowercase,
// which is its canonical form) and sorts the tfields by tkey.
func intlCanonicalizeTransformedExtension(body []string) []string {
	i := 0
	var out []string
	if i < len(body) && intlIsLanguageSubtag(body[i]) {
		n, _ := intlParseLanguageID(body, i)
		tlang := body[i:n]
		variants := []string(nil)
		// Sort the tlang's variants like the main language id's.
		j := 1
		if j < len(tlang) && intlIsScriptSubtag(tlang[j]) {
			j++
		}
		if j < len(tlang) && intlIsRegionSubtag(tlang[j]) {
			j++
		}
		variants = append(variants, tlang[j:]...)
		sort.Strings(variants)
		out = append(out, tlang[:j]...)
		out = append(out, variants...)
		i = n
	}
	type field struct {
		key    string
		values []string
	}
	var fields []field
	for i < len(body) {
		f := field{key: body[i]}
		i++
		for i < len(body) && len(body[i]) != 2 {
			f.values = append(f.values, body[i])
			i++
		}
		fields = append(fields, f)
	}
	sort.SliceStable(fields, func(a, b int) bool { return fields[a].key < fields[b].key })
	for _, f := range fields {
		out = append(out, f.key)
		out = append(out, f.values...)
	}
	return out
}

func intlDedupeSorted(s []string) []string {
	if len(s) < 2 {
		return s
	}
	out := s[:1]
	for _, v := range s[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

// ---- Abstract operations over VM values ----

// intlAbrupt is the tail of every Intl native's error path. The conversion
// helpers (getStringValueWithVM, toNumberWithVM, toLengthWithVM, ...) report
// a JS exception thrown inside ToPrimitive as ErrVMUnwinding: the VM is
// already unwinding towards the handler, so the native must return normally
// rather than surface the sentinel as a second, bogus "VM unwinding" Error.
// Every other error (TypeError/RangeError values, errors from Call/GetProperty)
// is returned as-is.
func intlAbrupt(err error) (vm.Value, error) {
	if err == ErrVMUnwinding {
		return vm.Undefined, nil
	}
	return vm.Undefined, err
}

// intlArg returns args[i] or undefined.
func intlArg(args []vm.Value, i int) vm.Value {
	if i < len(args) {
		return args[i]
	}
	return vm.Undefined
}

// intlCanonicalizeLocaleList implements ECMA-402 CanonicalizeLocaleList:
// undefined -> empty; a string -> a one-element list; otherwise ToObject the
// argument and walk it array-like, requiring each present element to be a
// String or Object, structurally valid, and canonicalizing + deduplicating.
// (Intl.Locale objects are not implemented, so objects go through ToString.)
func intlCanonicalizeLocaleList(vmInstance *vm.VM, locales vm.Value) ([]string, error) {
	if locales.Type() == vm.TypeUndefined {
		return nil, nil
	}
	var obj vm.Value
	if locales.Type() == vm.TypeString {
		obj = vm.NewArrayWithArgs([]vm.Value{locales})
	} else {
		o, err := vmInstance.ToObject(locales)
		if err != nil {
			return nil, err
		}
		obj = o
	}
	lenVal, err := vmInstance.GetProperty(obj, "length")
	if err != nil {
		return nil, err
	}
	length, err := toLengthWithVM(vmInstance, lenVal)
	if err != nil {
		return nil, err
	}
	var seen []string
	for k := 0; k < length; k++ {
		kValue, present, err := arrayLikeGet(vmInstance, obj, k)
		if err != nil {
			return nil, err
		}
		if !present {
			continue
		}
		if kValue.Type() != vm.TypeString && !kValue.IsObject() && !kValue.IsCallable() {
			return nil, vmInstance.NewTypeError("Language ID should be string or object.")
		}
		tag, err := getStringValueWithVM(vmInstance, kValue)
		if err != nil {
			return nil, err
		}
		if !intlIsStructurallyValidLanguageTag(tag) {
			return nil, vmInstance.NewRangeError(fmt.Sprintf("Incorrect locale information provided: %q", tag))
		}
		canonical := intlCanonicalizeUnicodeLocaleID(tag)
		dup := false
		for _, s := range seen {
			if s == canonical {
				dup = true
				break
			}
		}
		if !dup {
			seen = append(seen, canonical)
		}
	}
	return seen, nil
}

// intlStringList turns a Go string slice into a fresh, mutable JS Array -
// CreateArrayFromList.
func intlStringList(vmInstance *vm.VM, list []string) vm.Value {
	elems := make([]vm.Value, len(list))
	for i, s := range list {
		elems[i] = vm.NewString(s)
	}
	return vmInstance.NewArrayFromSlice(elems)
}

// intlGetOptionsObject implements ECMA-402 GetOptionsObject (used by the
// constructors): undefined -> a null-prototype object, an Object -> itself,
// anything else -> TypeError.
func intlGetOptionsObject(vmInstance *vm.VM, options vm.Value) (vm.Value, error) {
	if options.Type() == vm.TypeUndefined {
		return vm.NewObject(vm.Null), nil
	}
	if options.IsObject() || options.IsCallable() {
		return options, nil
	}
	return vm.Undefined, vmInstance.NewTypeError("Options must be an object or undefined")
}

// intlCoerceOptionsToObject implements ECMA-402 CoerceOptionsToObject (used
// by supportedLocalesOf): undefined -> a null-prototype object, otherwise
// ToObject.
func intlCoerceOptionsToObject(vmInstance *vm.VM, options vm.Value) (vm.Value, error) {
	if options.Type() == vm.TypeUndefined {
		return vm.NewObject(vm.Null), nil
	}
	return vmInstance.ToObject(options)
}

// intlGetStringOption implements ECMA-402 GetOption for a "string" option:
// Get the property, fall back when undefined, ToString it (an abrupt
// toString or a Symbol propagates as the error it is), and range-check it
// against allowed when allowed is non-nil.
func intlGetStringOption(vmInstance *vm.VM, options vm.Value, property, owner string, allowed []string, fallback string) (string, error) {
	value, err := vmInstance.GetProperty(options, property)
	if err != nil {
		return "", err
	}
	if value.Type() == vm.TypeUndefined {
		return fallback, nil
	}
	s, err := getStringValueWithVM(vmInstance, value)
	if err != nil {
		return "", err
	}
	if allowed != nil {
		ok := false
		for _, a := range allowed {
			if a == s {
				ok = true
				break
			}
		}
		if !ok {
			return "", vmInstance.NewRangeError(fmt.Sprintf("Value %s out of range for %s options property %s", s, owner, property))
		}
	}
	return s, nil
}

var intlLocaleMatchers = []string{"lookup", "best fit"}

// ---- Default locale and lookup matching ----

var (
	intlDefaultLocaleOnce  sync.Once
	intlDefaultLocaleValue string
)

// intlDefaultLocale implements DefaultLocale: the host's preferred locale,
// taken from the POSIX locale environment (LC_ALL, LC_MESSAGES, LANG - e.g.
// "en_US.UTF-8" -> "en-US") when it names an available locale, and "en-US"
// otherwise (including the "C"/"POSIX" locales). Computed once per process.
func intlDefaultLocale() string {
	intlDefaultLocaleOnce.Do(func() {
		intlDefaultLocaleValue = "en-US"
		for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
			v := os.Getenv(key)
			if v == "" {
				continue
			}
			if i := strings.IndexAny(v, ".@"); i >= 0 {
				v = v[:i]
			}
			v = strings.ReplaceAll(v, "_", "-")
			if intlIsStructurallyValidLanguageTag(v) {
				canonical := intlCanonicalizeUnicodeLocaleID(v)
				if available, ok := intlBestAvailableLocale(intlRemoveUnicodeExtensions(canonical)); ok {
					intlDefaultLocaleValue = available
				}
			}
			break
		}
	})
	return intlDefaultLocaleValue
}

// intlIsAvailableLocale is the stand-in for an Intl constructor's
// [[AvailableLocales]]: a canonical language(-Script)?(-REGION)? tag whose
// language subtag is a known ISO 639 code other than the "no language"
// placeholders. Variants, extensions and private use never match, so the
// lookup matcher truncates them off exactly as an ICU-backed engine would.
func intlIsAvailableLocale(locale string) bool {
	parts := strings.Split(locale, "-")
	if len(parts) > 3 {
		return false
	}
	switch parts[0] {
	case "und", "zxx", "mul", "mis":
		return false
	}
	if _, err := language.ParseBase(parts[0]); err != nil {
		return false
	}
	i := 1
	if i < len(parts) && intlIsScriptSubtag(parts[i]) {
		i++
	}
	if i < len(parts) && intlIsRegionSubtag(parts[i]) {
		i++
	}
	return i == len(parts)
}

// intlBestAvailableLocale implements BestAvailableLocale: try the locale,
// then successively drop its trailing subtag (skipping over a lone
// singleton) until an available locale is found.
func intlBestAvailableLocale(locale string) (string, bool) {
	candidate := locale
	for {
		if intlIsAvailableLocale(candidate) {
			return candidate, true
		}
		pos := strings.LastIndexByte(candidate, '-')
		if pos < 0 {
			return "", false
		}
		if pos >= 2 && candidate[pos-2] == '-' {
			pos -= 2
		}
		candidate = candidate[:pos]
	}
}

// intlRemoveUnicodeExtensions strips the -u- extension sequence from a
// canonical tag (the "noExtensionsLocale" of LookupMatcher).
func intlRemoveUnicodeExtensions(locale string) string {
	parts := strings.Split(locale, "-")
	out := make([]string, 0, len(parts))
	skipping := false
	for _, p := range parts {
		if len(p) == 1 {
			skipping = p == "u"
		}
		if !skipping {
			out = append(out, p)
		}
	}
	return strings.Join(out, "-")
}

// intlLookupMatcher implements LookupMatcher for a constructor with no
// relevant extension keys: the first requested locale (minus extensions)
// that has an available prefix wins, else the default locale. (BestFitMatcher
// is implementation-defined; this engine uses the same algorithm.)
func intlLookupMatcher(requested []string) string {
	for _, locale := range requested {
		if available, ok := intlBestAvailableLocale(intlRemoveUnicodeExtensions(locale)); ok {
			return available
		}
	}
	return intlDefaultLocale()
}

// intlLookupSupportedLocales implements LookupSupportedLocales: the
// requested locales (extensions intact) that have an available prefix.
func intlLookupSupportedLocales(requested []string) []string {
	var out []string
	for _, locale := range requested {
		if _, ok := intlBestAvailableLocale(intlRemoveUnicodeExtensions(locale)); ok {
			out = append(out, locale)
		}
	}
	return out
}

// intlSupportedLocalesOf implements the shared body of every
// Intl.<Constructor>.supportedLocalesOf(locales, options).
func intlSupportedLocalesOf(vmInstance *vm.VM, owner string, locales, options vm.Value) (vm.Value, error) {
	requested, err := intlCanonicalizeLocaleList(vmInstance, locales)
	if err != nil {
		return vm.Undefined, err
	}
	optionsObj, err := intlCoerceOptionsToObject(vmInstance, options)
	if err != nil {
		return vm.Undefined, err
	}
	if _, err := intlGetStringOption(vmInstance, optionsObj, "localeMatcher", owner, intlLocaleMatchers, "best fit"); err != nil {
		return vm.Undefined, err
	}
	return intlStringList(vmInstance, intlLookupSupportedLocales(requested)), nil
}

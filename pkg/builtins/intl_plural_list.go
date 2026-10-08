package builtins

import (
	"math"
	"strconv"
	"strings"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"

	"github.com/nooga/paserati/pkg/vm"
)

// ---- Intl.PluralRules ----

// intlPluralRules holds an Intl.PluralRules's slots; its digit options
// live in an embedded NumberFormat record, and selection uses x/text's
// CLDR plural rules.
type intlPluralRules struct {
	locale string
	typ    string // cardinal | ordinal
	digits *intlNumberFormat
	tag    language.Tag
}

func intlNewPluralRules(vmInstance *vm.VM, locales, optionsArg vm.Value) (*intlPluralRules, error) {
	const owner = "Intl.PluralRules"
	requested, err := intlCanonicalizeLocaleList(vmInstance, locales)
	if err != nil {
		return nil, err
	}
	options, err := intlCoerceOptionsToObject(vmInstance, optionsArg)
	if err != nil {
		return nil, err
	}
	if _, err := intlGetStringOption(vmInstance, options, "localeMatcher", owner, intlLocaleMatchers, "best fit"); err != nil {
		return nil, err
	}
	pr := &intlPluralRules{digits: &intlNumberFormat{notation: "standard", style: "decimal"}}
	if pr.typ, err = intlGetStringOption(vmInstance, options, "type", owner, []string{"cardinal", "ordinal"}, "cardinal"); err != nil {
		return nil, err
	}
	if err := pr.digits.setDigitOptions(vmInstance, options, 0, 3); err != nil {
		return nil, err
	}
	pr.locale = intlLookupMatcher(requested)
	if pr.tag, err = language.Parse(pr.locale); err != nil {
		pr.tag = language.AmericanEnglish
	}
	return pr, nil
}

func (pr *intlPluralRules) rules() *plural.Rules {
	if pr.typ == "ordinal" {
		return plural.Ordinal
	}
	return plural.Cardinal
}

// intlPluralFormNames orders the categories as pluralCategories lists them.
var intlPluralFormNames = []struct {
	form plural.Form
	name string
}{
	{plural.Zero, "zero"}, {plural.One, "one"}, {plural.Two, "two"},
	{plural.Few, "few"}, {plural.Many, "many"}, {plural.Other, "other"},
}

func intlPluralName(f plural.Form) string {
	for _, n := range intlPluralFormNames {
		if n.form == f {
			return n.name
		}
	}
	return "other"
}

// selectFloat is ResolvePlural: the operands of the number as it would be
// formatted with the rules' digit options.
func (pr *intlPluralRules) selectFloat(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return "other"
	}
	raw, _ := pr.digits.formatNumericToString(intlDecimalFromFloat(n))
	return intlPluralName(pr.matchDigits(raw.intDigits, raw.fracDigits))
}

func (pr *intlPluralRules) matchDigits(intDigits, frac string) plural.Form {
	mod := func(s string) int {
		if len(s) > 7 {
			s = s[len(s)-7:]
		}
		v, _ := strconv.Atoi(s)
		return v
	}
	trimmed := strings.TrimRight(frac, "0")
	return pr.rules().MatchPlural(pr.tag, mod(intDigits), len(frac), len(trimmed), mod(frac), mod(trimmed))
}

// categories probes the locale's rules for the categories it uses.
func (pr *intlPluralRules) categories() []string {
	seen := map[plural.Form]bool{}
	for i := 0; i <= 1000; i++ {
		seen[pr.matchDigits(strconv.Itoa(i), "")] = true
	}
	for _, i := range []string{"1000000", "2000000", "1000001"} {
		seen[pr.matchDigits(i, "")] = true
	}
	for _, f := range []string{"1", "5", "01", "10", "25", "50", "001"} {
		for i := 0; i <= 3; i++ {
			seen[pr.matchDigits(strconv.Itoa(i), f)] = true
		}
	}
	var out []string
	for _, n := range intlPluralFormNames {
		if seen[n.form] {
			out = append(out, n.name)
		}
	}
	return out
}

func installIntlPluralRules(vmInstance *vm.VM, intlObj *vm.PlainObject) {
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	vmInstance.IntlPluralRulesPrototype = protoVal

	ctor := vm.NewConstructorWithProps(0, false, "PluralRules", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			return vm.Undefined, vmInstance.NewTypeError("Constructor Intl.PluralRules requires 'new'")
		}
		objProto, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%Intl.PluralRules.prototype%")
		if err != nil {
			return intlAbrupt(err)
		}
		pr, err := intlNewPluralRules(vmInstance, intlArg(args, 0), intlArg(args, 1))
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(objProto).AsPlainObject()
		obj.SetInternalSlots(pr)
		return vm.NewValueFromPlainObject(obj), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", protoVal)
	ctorProps.SetOwnNonEnumerable("supportedLocalesOf", vm.NewNativeFunction(1, false, "supportedLocalesOf", func(args []vm.Value) (vm.Value, error) {
		return intlSupportedLocalesOf(vmInstance, "Intl.PluralRules", intlArg(args, 0), intlArg(args, 1))
	}))
	proto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(vmInstance, proto, "Intl.PluralRules")

	receiver := func(method string) (*intlPluralRules, error) {
		pr, ok := intlSlotsOf(vmInstance.GetThis()).(*intlPluralRules)
		if !ok {
			return nil, vmInstance.NewTypeError("Method Intl.PluralRules.prototype." + method + " called on incompatible receiver")
		}
		return pr, nil
	}
	proto.SetOwnNonEnumerable("select", vm.NewNativeFunction(1, false, "select", func(args []vm.Value) (vm.Value, error) {
		pr, err := receiver("select")
		if err != nil {
			return vm.Undefined, err
		}
		n, err := toNumberWithVM(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		return vm.NewString(pr.selectFloat(n)), nil
	}))
	proto.SetOwnNonEnumerable("selectRange", vm.NewNativeFunction(2, false, "selectRange", func(args []vm.Value) (vm.Value, error) {
		pr, err := receiver("selectRange")
		if err != nil {
			return vm.Undefined, err
		}
		start, end := intlArg(args, 0), intlArg(args, 1)
		if start.Type() == vm.TypeUndefined || end.Type() == vm.TypeUndefined {
			return vm.Undefined, vmInstance.NewTypeError("start and end are required")
		}
		x, err := toNumberWithVM(vmInstance, start)
		if err != nil {
			return intlAbrupt(err)
		}
		y, err := toNumberWithVM(vmInstance, end)
		if err != nil {
			return intlAbrupt(err)
		}
		if math.IsNaN(x) || math.IsNaN(y) {
			return vm.Undefined, vmInstance.NewRangeError("Invalid range: NaN")
		}
		// CLDR plural ranges resolve to the end's category in most locales.
		return vm.NewString(pr.selectFloat(y)), nil
	}))
	proto.SetOwnNonEnumerable("resolvedOptions", vm.NewNativeFunction(0, false, "resolvedOptions", func(args []vm.Value) (vm.Value, error) {
		pr, err := receiver("resolvedOptions")
		if err != nil {
			return vm.Undefined, err
		}
		d := pr.digits
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		num := func(k string, v int) { obj.SetOwn(k, vm.NumberValue(float64(v))) }
		obj.SetOwn("locale", vm.NewString(pr.locale))
		obj.SetOwn("type", vm.NewString(pr.typ))
		num("minimumIntegerDigits", d.minInt)
		if d.hasFD {
			num("minimumFractionDigits", d.minFD)
			num("maximumFractionDigits", d.maxFD)
		}
		if d.hasSD {
			num("minimumSignificantDigits", d.minSD)
			num("maximumSignificantDigits", d.maxSD)
		}
		obj.SetOwn("pluralCategories", intlStringList(vmInstance, pr.categories()))
		num("roundingIncrement", d.roundingIncr)
		obj.SetOwn("roundingMode", vm.NewString(d.roundingMode))
		obj.SetOwn("roundingPriority", vm.NewString(d.roundingPrio))
		obj.SetOwn("trailingZeroDisplay", vm.NewString(d.trailingZero))
		return vm.NewValueFromPlainObject(obj), nil
	}))
	intlObj.SetOwnNonEnumerable("PluralRules", ctor)
}

// ---- Intl.ListFormat ----

// intlListFormat holds an Intl.ListFormat's slots. Only English list
// patterns are available; other locales resolve to the default en-US.
type intlListFormat struct {
	locale, typ, style string
}

func intlNewListFormat(vmInstance *vm.VM, locales, optionsArg vm.Value) (*intlListFormat, error) {
	const owner = "Intl.ListFormat"
	requested, err := intlCanonicalizeLocaleList(vmInstance, locales)
	if err != nil {
		return nil, err
	}
	options, err := intlGetOptionsObject(vmInstance, optionsArg)
	if err != nil {
		return nil, err
	}
	if _, err := intlGetStringOption(vmInstance, options, "localeMatcher", owner, intlLocaleMatchers, "best fit"); err != nil {
		return nil, err
	}
	lf := &intlListFormat{}
	lf.locale, _ = intlResolveLocaleKeys(requested, intlIsEnglishLocale, nil, nil, nil)
	if lf.typ, err = intlGetStringOption(vmInstance, options, "type", owner, []string{"conjunction", "disjunction", "unit"}, "conjunction"); err != nil {
		return nil, err
	}
	if lf.style, err = intlGetStringOption(vmInstance, options, "style", owner, []string{"long", "short", "narrow"}, "long"); err != nil {
		return nil, err
	}
	return lf, nil
}

// parts implements CreatePartsFromList with English (CLDR) patterns.
func (lf *intlListFormat) parts(list []string) []intlPart {
	n := len(list)
	if n == 0 {
		return nil
	}
	var pair, middle, end string
	switch lf.typ {
	case "conjunction":
		switch lf.style {
		case "long":
			pair, middle, end = " and ", ", ", ", and "
		case "short":
			pair, middle, end = " & ", ", ", ", & "
		default:
			pair, middle, end = ", ", ", ", ", "
		}
	case "disjunction":
		pair, middle, end = " or ", ", ", ", or "
	default: // unit
		if lf.style == "narrow" {
			pair, middle, end = " ", " ", " "
		} else {
			pair, middle, end = ", ", ", ", ", "
		}
	}
	out := []intlPart{{"element", list[0]}}
	for i := 1; i < n; i++ {
		sep := middle
		switch {
		case n == 2:
			sep = pair
		case i == n-1:
			sep = end
		}
		out = append(out, intlPart{"literal", sep}, intlPart{"element", list[i]})
	}
	return out
}

// intlStringListFromIterable implements StringListFromIterable.
func intlStringListFromIterable(vmInstance *vm.VM, iterable vm.Value) ([]string, error) {
	if iterable.Type() == vm.TypeUndefined {
		return nil, nil
	}
	rec, err := getIterator(vmInstance, iterable)
	if err != nil {
		return nil, err
	}
	var list []string
	for {
		v, done, err := iteratorStepValue(vmInstance, rec)
		if err != nil {
			return nil, err
		}
		if done {
			return list, nil
		}
		if v.Type() != vm.TypeString {
			typeErr := vmInstance.NewTypeError("Iterable yielded " + v.ToString() + " which is not a string")
			return nil, iteratorClose(vmInstance, rec.iter, typeErr)
		}
		list = append(list, v.ToString())
	}
}

func installIntlListFormat(vmInstance *vm.VM, intlObj *vm.PlainObject) {
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	vmInstance.IntlListFormatPrototype = protoVal

	ctor := vm.NewConstructorWithProps(0, false, "ListFormat", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			return vm.Undefined, vmInstance.NewTypeError("Constructor Intl.ListFormat requires 'new'")
		}
		objProto, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%Intl.ListFormat.prototype%")
		if err != nil {
			return intlAbrupt(err)
		}
		lf, err := intlNewListFormat(vmInstance, intlArg(args, 0), intlArg(args, 1))
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(objProto).AsPlainObject()
		obj.SetInternalSlots(lf)
		return vm.NewValueFromPlainObject(obj), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", protoVal)
	ctorProps.SetOwnNonEnumerable("supportedLocalesOf", vm.NewNativeFunction(1, false, "supportedLocalesOf", func(args []vm.Value) (vm.Value, error) {
		return intlSupportedLocalesOfWith(vmInstance, "Intl.ListFormat", intlIsEnglishLocale, intlArg(args, 0), intlArg(args, 1))
	}))
	proto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(vmInstance, proto, "Intl.ListFormat")

	receiver := func(method string) (*intlListFormat, error) {
		lf, ok := intlSlotsOf(vmInstance.GetThis()).(*intlListFormat)
		if !ok {
			return nil, vmInstance.NewTypeError("Method Intl.ListFormat.prototype." + method + " called on incompatible receiver")
		}
		return lf, nil
	}
	proto.SetOwnNonEnumerable("format", vm.NewNativeFunction(1, false, "format", func(args []vm.Value) (vm.Value, error) {
		lf, err := receiver("format")
		if err != nil {
			return vm.Undefined, err
		}
		list, err := intlStringListFromIterable(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		return vm.NewString(intlJoinParts(lf.parts(list))), nil
	}))
	proto.SetOwnNonEnumerable("formatToParts", vm.NewNativeFunction(1, false, "formatToParts", func(args []vm.Value) (vm.Value, error) {
		lf, err := receiver("formatToParts")
		if err != nil {
			return vm.Undefined, err
		}
		list, err := intlStringListFromIterable(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		return intlPartsArray(vmInstance, lf.parts(list), nil), nil
	}))
	proto.SetOwnNonEnumerable("resolvedOptions", vm.NewNativeFunction(0, false, "resolvedOptions", func(args []vm.Value) (vm.Value, error) {
		lf, err := receiver("resolvedOptions")
		if err != nil {
			return vm.Undefined, err
		}
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		obj.SetOwn("locale", vm.NewString(lf.locale))
		obj.SetOwn("type", vm.NewString(lf.typ))
		obj.SetOwn("style", vm.NewString(lf.style))
		return vm.NewValueFromPlainObject(obj), nil
	}))
	intlObj.SetOwnNonEnumerable("ListFormat", ctor)
}

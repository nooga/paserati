package builtins

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/nooga/paserati/pkg/vm"
)

// intlCollator holds an Intl.Collator's resolved slots. Comparison uses
// x/text's CLDR collation for the locale; sensitivity, caseFirst and
// ignorePunctuation are layered on top of it.
type intlCollator struct {
	locale            string
	usage             string
	sensitivity       string
	ignorePunctuation bool
	collation         string
	numeric           bool
	caseFirst         string

	full, noCase *collate.Collator
	boundCompare vm.Value
	compareBound bool
}

func intlNewCollator(vmInstance *vm.VM, locales, optionsArg vm.Value) (*intlCollator, error) {
	const owner = "Intl.Collator"
	requested, err := intlCanonicalizeLocaleList(vmInstance, locales)
	if err != nil {
		return nil, err
	}
	options, err := intlCoerceOptionsToObject(vmInstance, optionsArg)
	if err != nil {
		return nil, err
	}
	c := &intlCollator{}
	if c.usage, err = intlGetStringOption(vmInstance, options, "usage", owner, []string{"sort", "search"}, "sort"); err != nil {
		return nil, err
	}
	if _, err := intlGetStringOption(vmInstance, options, "localeMatcher", owner, intlLocaleMatchers, "best fit"); err != nil {
		return nil, err
	}
	collation, err := intlGetStringOption(vmInstance, options, "collation", owner, nil, "\x00")
	if err != nil {
		return nil, err
	}
	if collation == "\x00" {
		collation = ""
	} else if !intlIsUnicodeTypeSequence(collation) {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid collation : %s", collation))
	}
	numericOpt := ""
	numericVal, err := vmInstance.GetProperty(options, "numeric")
	if err != nil {
		return nil, err
	}
	if numericVal.Type() != vm.TypeUndefined {
		numericOpt = "false"
		if numericVal.IsTruthy() {
			numericOpt = "true"
		}
	}
	caseFirstOpt, err := intlGetStringOption(vmInstance, options, "caseFirst", owner, []string{"upper", "lower", "false"}, "")
	if err != nil {
		return nil, err
	}

	locale, values := intlResolveLocaleKeys(requested, intlIsAvailableLocale, []string{"co", "kf", "kn"},
		map[string]string{"co": strings.ToLower(collation), "kf": caseFirstOpt, "kn": numericOpt},
		map[string]func(string) bool{
			// "standard" and "search" are never valid -u-co- values.
			"co": func(string) bool { return false },
			"kf": func(s string) bool { return s == "upper" || s == "lower" || s == "false" },
			"kn": func(s string) bool { return s == "true" || s == "false" },
		})
	c.locale = locale
	c.collation = "default"
	c.caseFirst = values["kf"]
	if c.caseFirst == "" {
		c.caseFirst = "false"
	}
	c.numeric = values["kn"] == "true"

	if c.sensitivity, err = intlGetStringOption(vmInstance, options, "sensitivity", owner, []string{"base", "accent", "case", "variant"}, "variant"); err != nil {
		return nil, err
	}
	ipVal, err := vmInstance.GetProperty(options, "ignorePunctuation")
	if err != nil {
		return nil, err
	}
	lang, _ := intlLanguageRegion(intlRemoveUnicodeExtensions(locale))
	c.ignorePunctuation = lang == "th" // CLDR locale default
	if ipVal.Type() != vm.TypeUndefined {
		c.ignorePunctuation = ipVal.IsTruthy()
	}

	tag, err := language.Parse(intlRemoveUnicodeExtensions(locale))
	if err != nil {
		tag = language.Und
	}
	var opts []collate.Option
	if c.numeric {
		opts = append(opts, collate.Numeric)
	}
	c.full = collate.New(tag, opts...)
	c.noCase = collate.New(tag, append(opts, collate.IgnoreCase)...)
	return c, nil
}

var intlStripMarks = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

func intlStripDiacritics(s string) string {
	out, _, err := transform.String(intlStripMarks, s)
	if err != nil {
		return s
	}
	return out
}

func intlStripPunctuation(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r) && r != '+' && r != '<' && r != '=' && r != '>' {
			return -1
		}
		return r
	}, s)
}

// compare implements the collator's comparison function.
func (c *intlCollator) compare(x, y string) int {
	if c.ignorePunctuation {
		x, y = intlStripPunctuation(x), intlStripPunctuation(y)
	}
	switch c.sensitivity {
	case "base":
		return c.noCase.CompareString(intlStripDiacritics(x), intlStripDiacritics(y))
	case "accent":
		return c.noCase.CompareString(x, y)
	case "case":
		x, y = intlStripDiacritics(x), intlStripDiacritics(y)
	}
	r := c.full.CompareString(x, y)
	if r != 0 && c.caseFirst == "upper" && c.noCase.CompareString(x, y) == 0 {
		// Differing only in case: uppercase sorts first.
		return -r
	}
	return r
}

func installIntlCollator(vmInstance *vm.VM, intlObj *vm.PlainObject) {
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	vmInstance.IntlCollatorPrototype = protoVal

	var ctor vm.Value
	ctor = vm.NewConstructorWithProps(0, false, "Collator", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			newTarget = ctor
		}
		objProto, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%Intl.Collator.prototype%")
		if err != nil {
			return intlAbrupt(err)
		}
		c, err := intlNewCollator(vmInstance, intlArg(args, 0), intlArg(args, 1))
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(objProto).AsPlainObject()
		obj.SetInternalSlots(c)
		return vm.NewValueFromPlainObject(obj), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", protoVal)
	ctorProps.SetOwnNonEnumerable("supportedLocalesOf", vm.NewNativeFunction(1, false, "supportedLocalesOf", func(args []vm.Value) (vm.Value, error) {
		return intlSupportedLocalesOf(vmInstance, "Intl.Collator", intlArg(args, 0), intlArg(args, 1))
	}))
	proto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(vmInstance, proto, "Intl.Collator")

	receiver := func(method string) (*intlCollator, error) {
		c, ok := intlSlotsOf(vmInstance.GetThis()).(*intlCollator)
		if !ok {
			return nil, vmInstance.NewTypeError("Method Intl.Collator.prototype." + method + " called on incompatible receiver")
		}
		return c, nil
	}

	compareGetter := vm.NewNativeFunction(0, false, "get compare", func(args []vm.Value) (vm.Value, error) {
		c, err := receiver("compare")
		if err != nil {
			return vm.Undefined, err
		}
		if !c.compareBound {
			c.compareBound = true
			c.boundCompare = vm.NewNativeFunction(2, false, "", func(args []vm.Value) (vm.Value, error) {
				x, err := getStringValueWithVM(vmInstance, intlArg(args, 0))
				if err != nil {
					return intlAbrupt(err)
				}
				y, err := getStringValueWithVM(vmInstance, intlArg(args, 1))
				if err != nil {
					return intlAbrupt(err)
				}
				return vm.NumberValue(float64(c.compare(x, y))), nil
			})
		}
		return c.boundCompare, nil
	})
	enumerable, configurable := false, true
	proto.DefineAccessorProperty("compare", compareGetter, true, vm.Undefined, false, &enumerable, &configurable)

	proto.SetOwnNonEnumerable("resolvedOptions", vm.NewNativeFunction(0, false, "resolvedOptions", func(args []vm.Value) (vm.Value, error) {
		c, err := receiver("resolvedOptions")
		if err != nil {
			return vm.Undefined, err
		}
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		obj.SetOwn("locale", vm.NewString(c.locale))
		obj.SetOwn("usage", vm.NewString(c.usage))
		obj.SetOwn("sensitivity", vm.NewString(c.sensitivity))
		obj.SetOwn("ignorePunctuation", vm.BooleanValue(c.ignorePunctuation))
		obj.SetOwn("collation", vm.NewString(c.collation))
		obj.SetOwn("numeric", vm.BooleanValue(c.numeric))
		obj.SetOwn("caseFirst", vm.NewString(c.caseFirst))
		return vm.NewValueFromPlainObject(obj), nil
	}))

	intlObj.SetOwnNonEnumerable("Collator", ctor)
}

// intlCollatorFor returns a collator for localeCompare, reusing one per
// locale argument when there are no options (the common case). The cache
// lives on the VM: x/text collators keep scratch buffers, so they must not
// be shared across VMs running on different goroutines.
func intlCollatorFor(vmInstance *vm.VM, locales, options vm.Value) (*intlCollator, error) {
	cacheable := options.Type() == vm.TypeUndefined && (locales.Type() == vm.TypeUndefined || locales.Type() == vm.TypeString)
	key := ""
	cache, _ := vmInstance.IntlCache.(map[string]*intlCollator)
	if cacheable {
		if locales.Type() == vm.TypeString {
			key = locales.ToString()
		}
		if c, ok := cache[key]; ok {
			return c, nil
		}
	}
	c, err := intlNewCollator(vmInstance, locales, options)
	if err != nil {
		return nil, err
	}
	if cacheable && len(cache) < 64 {
		if cache == nil {
			cache = map[string]*intlCollator{}
			vmInstance.IntlCache = cache
		}
		cache[key] = c
	}
	return c, nil
}

package builtins

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/vm"
)

// intlNumberFormat holds an Intl.NumberFormat's resolved internal slots.
type intlNumberFormat struct {
	locale          string
	numberingSystem string
	style           string
	currency        string
	currencyDisplay string
	currencySign    string
	unit            string
	unitDisplay     string
	notation        string
	compactDisplay  string
	minInt          int
	minFD, maxFD    int
	minSD, maxSD    int
	hasSD, hasFD    bool   // which digit fields resolvedOptions reports
	roundingType    string // fractionDigits | significantDigits | morePrecision | lessPrecision
	roundingIncr    int
	roundingMode    string
	roundingPrio    string
	trailingZero    string
	useGrouping     string // "always" | "auto" | "min2" | "" (false)
	signDisplay     string
	boundFormat     vm.Value
	formatBound     bool
}

// ---- Numbering systems ----

// intlDigitZeros maps each simple-digit numbering system (ECMA-402 Table 14)
// to its digit zero; the other nine digits follow it.
var intlDigitZeros = map[string]rune{
	"adlm": 0x1E950, "ahom": 0x11730, "arab": 0x660, "arabext": 0x6F0, "bali": 0x1B50, "beng": 0x9E6,
	"bhks": 0x11C50, "brah": 0x11066, "cakm": 0x11136, "cham": 0xAA50, "deva": 0x966, "diak": 0x11950,
	"fullwide": 0xFF10, "gong": 0x11DA0, "gonm": 0x11D50, "gujr": 0xAE6, "guru": 0xA66, "hmng": 0x16B50,
	"hmnp": 0x1E140, "java": 0xA9D0, "kali": 0xA900, "kawi": 0x11F50, "khmr": 0x17E0, "knda": 0xCE6,
	"lana": 0x1A80, "lanatham": 0x1A90, "laoo": 0xED0, "latn": '0', "lepc": 0x1C40, "limb": 0x1946,
	"mathbold": 0x1D7CE, "mathdbl": 0x1D7D8, "mathmono": 0x1D7F6, "mathsanb": 0x1D7EC, "mathsans": 0x1D7E2,
	"mlym": 0xD66, "modi": 0x11650, "mong": 0x1810, "mroo": 0x16A60, "mtei": 0xABF0, "mymr": 0x1040,
	"mymrshan": 0x1090, "mymrtlng": 0xA9F0, "nagm": 0x1E4F0, "newa": 0x11450, "nkoo": 0x7C0, "olck": 0x1C50,
	"orya": 0xB66, "osma": 0x104A0, "rohg": 0x10D30, "saur": 0xA8D0, "segment": 0x1FBF0, "shrd": 0x111D0,
	"sind": 0x112F0, "sinh": 0xDE6, "sora": 0x110F0, "sund": 0x1BB0, "takr": 0x116C0, "talu": 0x19D0,
	"tamldec": 0xBE6, "telu": 0xC66, "thai": 0xE50, "tibt": 0xF20, "tirh": 0x114D0, "tnsa": 0x16AC0,
	"vaii": 0xA620, "wara": 0x118E0, "wcho": 0x1E2F0,
}

const intlHanidec = "〇一二三四五六七八九"

func intlIsSupportedNumberingSystem(nu string) bool {
	_, ok := intlDigitZeros[nu]
	return ok || nu == "hanidec"
}

// intlTransliterate rewrites ASCII digits into the numbering system's.
func intlTransliterate(s, nu string) string {
	if nu == "latn" || nu == "" {
		return s
	}
	var digits []rune
	if nu == "hanidec" {
		digits = []rune(intlHanidec)
	} else if zero, ok := intlDigitZeros[nu]; ok {
		for i := rune(0); i < 10; i++ {
			digits = append(digits, zero+i)
		}
	} else {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(digits[r-'0'])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// intlIsUnicodeTypeSequence matches the `type` nonterminal: (3*8alphanum)
// separated by "-".
func intlIsUnicodeTypeSequence(s string) bool {
	if s == "" {
		return false
	}
	for _, part := range strings.Split(s, "-") {
		if len(part) < 3 || len(part) > 8 || !intlAll(part, intlIsAlnum) {
			return false
		}
	}
	return true
}

// ---- Locale resolution with a Unicode extension key ----

// intlUnicodeExtensionValue returns the value of key in locale's -u-
// extension ("" when the keyword is present with no value, ok false when
// absent).
func intlUnicodeExtensionValue(locale, key string) (string, bool) {
	parts := strings.Split(locale, "-")
	inU := false
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) == 1 {
			inU = p == "u"
			continue
		}
		if inU && len(p) == 2 && p == key {
			var vals []string
			for j := i + 1; j < len(parts) && len(parts[j]) > 2; j++ {
				vals = append(vals, parts[j])
			}
			return strings.Join(vals, "-"), true
		}
	}
	return "", false
}

// intlResolveLocaleKey is ResolveLocale for one relevant extension key:
// the matched available locale, plus the key's value from the request's
// -u- extension when supported, overridden by optionValue when that is
// supported; the extension is echoed in the locale only when it was used.
func intlResolveLocaleKey(requested []string, key, optionValue string, supported func(string) bool, fallback string) (locale, value string) {
	found := ""
	foundRequested := ""
	for _, r := range requested {
		if available, ok := intlBestAvailableLocale(intlRemoveUnicodeExtensions(r)); ok {
			found, foundRequested = available, r
			break
		}
	}
	if found == "" {
		found = intlDefaultLocale()
	}
	value = fallback
	supportedExt := ""
	if foundRequested != "" {
		if v, ok := intlUnicodeExtensionValue(foundRequested, key); ok && v != "" && supported(v) {
			value, supportedExt = v, "-u-"+key+"-"+v
		}
	}
	if optionValue != "" && supported(optionValue) && optionValue != value {
		value, supportedExt = optionValue, ""
	}
	return found + supportedExt, value
}

// ---- Option helpers ----

// intlGetNumberOption is GetNumberOption / DefaultNumberOption.
func intlGetNumberOption(vmInstance *vm.VM, options vm.Value, property string, minimum, maximum int, fallback int) (int, bool, error) {
	value, err := vmInstance.GetProperty(options, property)
	if err != nil {
		return 0, false, err
	}
	return intlDefaultNumberOption(vmInstance, value, property, minimum, maximum, fallback)
}

func intlDefaultNumberOption(vmInstance *vm.VM, value vm.Value, property string, minimum, maximum int, fallback int) (int, bool, error) {
	if value.Type() == vm.TypeUndefined {
		return fallback, false, nil
	}
	f, err := toNumberWithVM(vmInstance, value)
	if err != nil {
		return 0, false, err
	}
	if math.IsNaN(f) || f < float64(minimum) || f > float64(maximum) {
		return 0, false, vmInstance.NewRangeError(fmt.Sprintf("%s value is out of range.", property))
	}
	return int(math.Floor(f)), true, nil
}

var intlRoundingIncrements = []int{1, 2, 5, 10, 20, 25, 50, 100, 200, 250, 500, 1000, 2000, 2500, 5000}

// intlIsWellFormedCurrencyCode: three ASCII letters.
func intlIsWellFormedCurrencyCode(s string) bool {
	return len(s) == 3 && intlAll(s, intlIsAlpha)
}

// intlSanctionedUnits is the ECMA-402 list of simple units.
var intlSanctionedUnits = map[string]bool{
	"acre": true, "bit": true, "byte": true, "celsius": true, "centimeter": true, "day": true, "degree": true,
	"fahrenheit": true, "fluid-ounce": true, "foot": true, "gallon": true, "gigabit": true, "gigabyte": true,
	"gram": true, "hectare": true, "hour": true, "inch": true, "kilobit": true, "kilobyte": true, "kilogram": true,
	"kilometer": true, "liter": true, "megabit": true, "megabyte": true, "meter": true, "microsecond": true,
	"mile": true, "mile-scandinavian": true, "milliliter": true, "millimeter": true, "millisecond": true,
	"minute": true, "month": true, "nanosecond": true, "ounce": true, "percent": true, "petabyte": true,
	"pound": true, "second": true, "stone": true, "terabit": true, "terabyte": true, "week": true, "yard": true,
	"year": true,
}

// intlIsWellFormedUnitIdentifier: a sanctioned unit, or two joined by "-per-".
func intlIsWellFormedUnitIdentifier(s string) bool {
	if intlSanctionedUnits[s] {
		return true
	}
	num, den, ok := strings.Cut(s, "-per-")
	return ok && intlSanctionedUnits[num] && intlSanctionedUnits[den]
}

// ---- InitializeNumberFormat ----

func intlNewNumberFormat(vmInstance *vm.VM, locales, optionsArg vm.Value) (*intlNumberFormat, error) {
	requested, err := intlCanonicalizeLocaleList(vmInstance, locales)
	if err != nil {
		return nil, err
	}
	options, err := intlCoerceOptionsToObject(vmInstance, optionsArg)
	if err != nil {
		return nil, err
	}
	const owner = "Intl.NumberFormat"
	if _, err := intlGetStringOption(vmInstance, options, "localeMatcher", owner, intlLocaleMatchers, "best fit"); err != nil {
		return nil, err
	}
	nuOption, err := intlGetStringOption(vmInstance, options, "numberingSystem", owner, nil, "\x00")
	if err != nil {
		return nil, err
	}
	if nuOption == "\x00" {
		nuOption = ""
	} else if !intlIsUnicodeTypeSequence(nuOption) {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid numberingSystem : %s", nuOption))
	}
	nf := &intlNumberFormat{}
	nf.locale, nf.numberingSystem = intlResolveLocaleKey(requested, "nu", strings.ToLower(nuOption), intlIsSupportedNumberingSystem, "latn")

	// SetNumberFormatUnitOptions
	if nf.style, err = intlGetStringOption(vmInstance, options, "style", owner, []string{"decimal", "percent", "currency", "unit"}, "decimal"); err != nil {
		return nil, err
	}
	cur, err := intlGetStringOption(vmInstance, options, "currency", owner, nil, "\x00")
	if err != nil {
		return nil, err
	}
	if cur == "\x00" {
		if nf.style == "currency" {
			return nil, vmInstance.NewTypeError("Currency code is required with currency style.")
		}
	} else if !intlIsWellFormedCurrencyCode(cur) {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid currency code : %s", cur))
	}
	currencyDisplay, err := intlGetStringOption(vmInstance, options, "currencyDisplay", owner, []string{"code", "symbol", "narrowSymbol", "name"}, "symbol")
	if err != nil {
		return nil, err
	}
	currencySign, err := intlGetStringOption(vmInstance, options, "currencySign", owner, []string{"standard", "accounting"}, "standard")
	if err != nil {
		return nil, err
	}
	unit, err := intlGetStringOption(vmInstance, options, "unit", owner, nil, "\x00")
	if err != nil {
		return nil, err
	}
	if unit == "\x00" {
		if nf.style == "unit" {
			return nil, vmInstance.NewTypeError("Unit is required with unit style.")
		}
	} else if !intlIsWellFormedUnitIdentifier(unit) {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid unit argument for Intl.NumberFormat() '%s'", unit))
	}
	unitDisplay, err := intlGetStringOption(vmInstance, options, "unitDisplay", owner, []string{"short", "narrow", "long"}, "short")
	if err != nil {
		return nil, err
	}
	if nf.style == "currency" {
		nf.currency, nf.currencyDisplay, nf.currencySign = strings.ToUpper(cur), currencyDisplay, currencySign
	}
	if nf.style == "unit" {
		nf.unit, nf.unitDisplay = unit, unitDisplay
	}

	if nf.notation, err = intlGetStringOption(vmInstance, options, "notation", owner, []string{"standard", "scientific", "engineering", "compact"}, "standard"); err != nil {
		return nil, err
	}
	mnfdDefault, mxfdDefault := 0, 3
	if nf.style == "currency" && nf.notation == "standard" {
		d := intlCurrencyDigits(nf.currency)
		mnfdDefault, mxfdDefault = d, d
	} else if nf.style == "percent" {
		mxfdDefault = 0
	}
	if err := nf.setDigitOptions(vmInstance, options, mnfdDefault, mxfdDefault); err != nil {
		return nil, err
	}
	if nf.compactDisplay, err = intlGetStringOption(vmInstance, options, "compactDisplay", owner, []string{"short", "long"}, "short"); err != nil {
		return nil, err
	}
	defaultGrouping := "auto"
	if nf.notation == "compact" {
		defaultGrouping = "min2"
	}
	if nf.useGrouping, err = intlGetBooleanOrStringOption(vmInstance, options, "useGrouping", owner, []string{"min2", "auto", "always", "true", "false"}, "always", "", defaultGrouping); err != nil {
		return nil, err
	}
	if nf.signDisplay, err = intlGetStringOption(vmInstance, options, "signDisplay", owner, []string{"auto", "never", "always", "exceptZero", "negative"}, "auto"); err != nil {
		return nil, err
	}
	return nf, nil
}

// intlGetBooleanOrStringOption implements GetBooleanOrStringOption.
func intlGetBooleanOrStringOption(vmInstance *vm.VM, options vm.Value, property, owner string, values []string, trueValue, falsyValue, fallback string) (string, error) {
	value, err := vmInstance.GetProperty(options, property)
	if err != nil {
		return "", err
	}
	if value.Type() == vm.TypeUndefined {
		return fallback, nil
	}
	if value.Type() == vm.TypeBoolean && value.AsBoolean() {
		return trueValue, nil
	}
	if !value.IsTruthy() {
		return falsyValue, nil
	}
	s, err := getStringValueWithVM(vmInstance, value)
	if err != nil {
		return "", err
	}
	if s == "true" || s == "false" {
		return fallback, nil
	}
	for _, v := range values {
		if v == s {
			return s, nil
		}
	}
	return "", vmInstance.NewRangeError(fmt.Sprintf("Value %s out of range for %s options property %s", s, owner, property))
}

// setDigitOptions implements SetNumberFormatDigitOptions.
func (nf *intlNumberFormat) setDigitOptions(vmInstance *vm.VM, options vm.Value, mnfdDefault, mxfdDefault int) error {
	const owner = "Intl.NumberFormat"
	var err error
	if nf.minInt, _, err = intlGetNumberOption(vmInstance, options, "minimumIntegerDigits", 1, 21, 1); err != nil {
		return err
	}
	get := func(name string) (vm.Value, error) { return vmInstance.GetProperty(options, name) }
	mnfd, err := get("minimumFractionDigits")
	if err != nil {
		return err
	}
	mxfd, err := get("maximumFractionDigits")
	if err != nil {
		return err
	}
	mnsd, err := get("minimumSignificantDigits")
	if err != nil {
		return err
	}
	mxsd, err := get("maximumSignificantDigits")
	if err != nil {
		return err
	}
	if nf.roundingIncr, _, err = intlGetNumberOption(vmInstance, options, "roundingIncrement", 1, 5000, 1); err != nil {
		return err
	}
	validIncr := false
	for _, v := range intlRoundingIncrements {
		validIncr = validIncr || v == nf.roundingIncr
	}
	if !validIncr {
		return vmInstance.NewRangeError("roundingIncrement value is out of range.")
	}
	if nf.roundingMode, err = intlGetStringOption(vmInstance, options, "roundingMode", owner,
		[]string{"ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"}, "halfExpand"); err != nil {
		return err
	}
	if nf.roundingPrio, err = intlGetStringOption(vmInstance, options, "roundingPriority", owner, []string{"auto", "morePrecision", "lessPrecision"}, "auto"); err != nil {
		return err
	}
	if nf.trailingZero, err = intlGetStringOption(vmInstance, options, "trailingZeroDisplay", owner, []string{"auto", "stripIfInteger"}, "auto"); err != nil {
		return err
	}
	if nf.roundingIncr != 1 {
		mxfdDefault = mnfdDefault
	}
	hasSd := mnsd.Type() != vm.TypeUndefined || mxsd.Type() != vm.TypeUndefined
	hasFd := mnfd.Type() != vm.TypeUndefined || mxfd.Type() != vm.TypeUndefined
	needSd, needFd := true, true
	if nf.roundingPrio == "auto" {
		needSd = hasSd
		if needSd || (!hasFd && nf.notation == "compact") {
			needFd = false
		}
	}
	if needSd {
		if hasSd {
			if nf.minSD, _, err = intlDefaultNumberOption(vmInstance, mnsd, "minimumSignificantDigits", 1, 21, 1); err != nil {
				return err
			}
			if nf.maxSD, _, err = intlDefaultNumberOption(vmInstance, mxsd, "maximumSignificantDigits", nf.minSD, 21, 21); err != nil {
				return err
			}
		} else {
			nf.minSD, nf.maxSD = 1, 21
		}
	}
	if needFd {
		if hasFd {
			lo, loSet, err := intlDefaultNumberOption(vmInstance, mnfd, "minimumFractionDigits", 0, 100, 0)
			if err != nil {
				return err
			}
			hi, hiSet, err := intlDefaultNumberOption(vmInstance, mxfd, "maximumFractionDigits", 0, 100, 0)
			if err != nil {
				return err
			}
			switch {
			case !loSet:
				lo = min(mnfdDefault, hi)
			case !hiSet:
				hi = max(mxfdDefault, lo)
			case lo > hi:
				return vmInstance.NewRangeError("maximumFractionDigits value is out of range.")
			}
			nf.minFD, nf.maxFD = lo, hi
		} else {
			nf.minFD, nf.maxFD = mnfdDefault, mxfdDefault
		}
	}
	switch {
	case !needSd && !needFd:
		nf.minFD, nf.maxFD, nf.minSD, nf.maxSD = 0, 0, 1, 2
		nf.roundingType, nf.roundingPrio = "morePrecision", "morePrecision"
		needSd, needFd = true, true
	case nf.roundingPrio == "auto":
		if hasSd {
			nf.roundingType = "significantDigits"
		} else {
			nf.roundingType = "fractionDigits"
		}
	default:
		nf.roundingType = nf.roundingPrio
	}
	nf.hasSD, nf.hasFD = needSd, needFd
	if nf.roundingIncr != 1 {
		if nf.roundingType != "fractionDigits" {
			return vmInstance.NewTypeError("roundingIncrement can only be used with fractionDigits rounding.")
		}
		if nf.maxFD != nf.minFD {
			return vmInstance.NewRangeError("maximumFractionDigits and minimumFractionDigits must be equal with roundingIncrement.")
		}
	}
	return nil
}

// ---- FormatNumericToString ----

func (nf *intlNumberFormat) formatNumericToString(x intlDecimal) (intlRawNumber, intlDecimal) {
	var raw intlRawNumber
	var rounded intlDecimal
	switch nf.roundingType {
	case "significantDigits":
		raw, rounded = x.toRawPrecision(nf.minSD, nf.maxSD, nf.roundingMode)
	case "fractionDigits":
		raw, rounded = x.toRawFixed(nf.minFD, nf.maxFD, int64(nf.roundingIncr), nf.roundingMode)
	default:
		sRaw, sRounded := x.toRawPrecision(nf.minSD, nf.maxSD, nf.roundingMode)
		fRaw, fRounded := x.toRawFixed(nf.minFD, nf.maxFD, int64(nf.roundingIncr), nf.roundingMode)
		pickS := sRaw.magnitude <= fRaw.magnitude
		if nf.roundingType == "lessPrecision" {
			pickS = !pickS || sRaw.magnitude == fRaw.magnitude
		}
		if pickS {
			raw, rounded = sRaw, sRounded
		} else {
			raw, rounded = fRaw, fRounded
		}
	}
	if nf.trailingZero == "stripIfInteger" && strings.Trim(raw.fracDigits, "0") == "" {
		raw.fracDigits = ""
	}
	if len(raw.intDigits) < nf.minInt {
		raw.intDigits = strings.Repeat("0", nf.minInt-len(raw.intDigits)) + raw.intDigits
	}
	return raw, rounded
}

// ---- Compact and scientific notation ----

var intlCompactShort = []string{"", "K", "M", "B", "T"}
var intlCompactLong = []string{"", "thousand", "million", "billion", "trillion"}

func (nf *intlNumberFormat) exponentFor(x intlDecimal) int {
	if x.isZero() || x.nan || x.inf {
		return 0
	}
	m := x.magnitude()
	switch nf.notation {
	case "scientific":
		return m
	case "engineering":
		return intlFloorDiv(m, 3) * 3
	case "compact":
		if m < 3 {
			return 0
		}
		return min(m/3*3, 12)
	}
	return 0
}

func intlFloorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// ---- FormatNumericToParts ----

func (nf *intlNumberFormat) partsFor(x intlDecimal) []intlPart {
	sym := intlNumberSymbolsFor(intlRemoveUnicodeExtensions(nf.locale))
	if nf.style == "percent" {
		x = x.shift(2)
	}

	var numParts []intlPart
	var raw intlRawNumber
	rounded := x
	exponent := 0
	switch {
	case x.nan:
		numParts = []intlPart{{"nan", "NaN"}}
	case x.inf:
		numParts = []intlPart{{"infinity", "∞"}}
	default:
		exponent = nf.exponentFor(x)
		raw, rounded = nf.formatNumericToString(x.shift(-exponent))
		if nf.notation != "standard" && !rounded.isZero() {
			// Rounding can carry into the next exponent (9.99E2 -> 1E3, 999.9K -> 1M).
			mag := rounded.magnitude()
			if nf.notation == "scientific" && mag >= 1 ||
				nf.notation == "engineering" && mag >= 3 ||
				nf.notation == "compact" && mag >= 3 && exponent < 12 {
				exponent = nf.exponentFor(rounded.shift(exponent))
				raw, rounded = nf.formatNumericToString(x.shift(-exponent))
			}
		}
		grouping := nf.useGrouping
		if nf.notation == "scientific" || nf.notation == "engineering" {
			grouping = ""
		}
		groups := intlGroup(raw.intDigits, sym, grouping)
		for i, g := range groups {
			if i > 0 {
				numParts = append(numParts, intlPart{"group", sym.group})
			}
			numParts = append(numParts, intlPart{"integer", intlTransliterate(g, nf.numberingSystem)})
		}
		if raw.fracDigits != "" {
			numParts = append(numParts, intlPart{"decimal", sym.decimal}, intlPart{"fraction", intlTransliterate(raw.fracDigits, nf.numberingSystem)})
		}
		switch nf.notation {
		case "scientific", "engineering":
			numParts = append(numParts, intlPart{"exponentSeparator", "E"})
			if exponent < 0 {
				numParts = append(numParts, intlPart{"exponentMinusSign", sym.minus})
			}
			numParts = append(numParts, intlPart{"exponentInteger", intlTransliterate(strconv.Itoa(abs(exponent)), nf.numberingSystem)})
		case "compact":
			if exponent > 0 {
				if nf.compactDisplay == "long" {
					numParts = append(numParts, intlPart{"literal", " "}, intlPart{"compact", intlCompactLong[exponent/3]})
				} else {
					numParts = append(numParts, intlPart{"compact", intlCompactShort[exponent/3]})
				}
			}
		}
	}

	// Sign.
	isZero := x.nan || (!x.inf && rounded.isZero())
	var sign *intlPart
	minus := intlPart{"minusSign", sym.minus}
	plus := intlPart{"plusSign", sym.plus}
	switch nf.signDisplay {
	case "auto":
		if x.neg && !x.nan {
			sign = &minus
		}
	case "always":
		if x.neg && !x.nan {
			sign = &minus
		} else {
			sign = &plus
		}
	case "exceptZero":
		if !isZero {
			if x.neg {
				sign = &minus
			} else {
				sign = &plus
			}
		}
	case "negative":
		if x.neg && !isZero {
			sign = &minus
		}
	}
	accounting := nf.style == "currency" && nf.currencySign == "accounting" && sign != nil && sign.typ == "minusSign" &&
		intlAccountingParens[intlLanguageOf(nf.locale)]
	if accounting {
		sign = nil
	}

	var out []intlPart
	addSign := func() {
		if sign != nil {
			out = append(out, *sign)
		}
	}
	switch nf.style {
	case "percent":
		addSign()
		if sym.percentPrefix != "" {
			out = append(out, intlSplitAffix(sym.percentPrefix, "percentSign", "%")...)
		}
		out = append(out, numParts...)
		if sym.percentSuffix != "" {
			out = append(out, intlSplitAffix(sym.percentSuffix, "percentSign", "%")...)
		}
	case "currency":
		out = nf.currencyParts(sym, numParts, raw, addSign, &out)
	case "unit":
		addSign()
		out = append(out, nf.unitParts(numParts, raw, x)...)
	default:
		addSign()
		out = append(out, numParts...)
	}
	if accounting {
		out = append(append([]intlPart{{"literal", "("}}, out...), intlPart{"literal", ")"})
	}
	return out
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// intlSplitAffix turns a pattern affix like "\u00a0%" into literal and
// symbol parts.
func intlSplitAffix(affix, typ, symbol string) []intlPart {
	i := strings.Index(affix, symbol)
	if i < 0 {
		return []intlPart{{"literal", affix}}
	}
	var parts []intlPart
	if i > 0 {
		parts = append(parts, intlPart{"literal", affix[:i]})
	}
	parts = append(parts, intlPart{typ, symbol})
	if rest := affix[i+len(symbol):]; rest != "" {
		parts = append(parts, intlPart{"literal", rest})
	}
	return parts
}

func (nf *intlNumberFormat) currencyParts(sym *intlNumberSymbols, numParts []intlPart, raw intlRawNumber, addSign func(), out *[]intlPart) []intlPart {
	if nf.currencyDisplay == "name" {
		addSign()
		*out = append(*out, numParts...)
		name := nf.currency
		if names, ok := intlCurrencyNames[nf.currency]; ok {
			name = names[1]
			if raw.intDigits == "1" && raw.fracDigits == "" {
				name = names[0]
			}
		}
		return append(*out, intlPart{"literal", " "}, intlPart{"currency", name})
	}
	display := nf.currency
	switch nf.currencyDisplay {
	case "symbol":
		display = intlCurrencySymbol(intlRemoveUnicodeExtensions(nf.locale), nf.currency, false)
	case "narrowSymbol":
		display = intlCurrencySymbol(intlRemoveUnicodeExtensions(nf.locale), nf.currency, true)
	}
	space := sym.currencySpace
	if space == "" && sym.currencyBefore {
		// ICU currency spacing: a symbol ending in a letter is set off from
		// the digits ("USD 1.00", "CA$1.00" stays tight).
		if r := []rune(display); len(r) > 0 && intlIsLetterRune(r[len(r)-1]) {
			space = "\u00a0"
		}
	}
	if sym.currencyBefore {
		addSign()
		*out = append(*out, intlPart{"currency", display})
		if space != "" {
			*out = append(*out, intlPart{"literal", space})
		}
		return append(*out, numParts...)
	}
	addSign()
	*out = append(*out, numParts...)
	if space != "" {
		*out = append(*out, intlPart{"literal", space})
	}
	return append(*out, intlPart{"currency", display})
}

func intlIsLetterRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// ---- Units (English display data) ----

type intlUnitForms struct {
	short1, shortN string // short singular/plural ("hr", "hr"; "day", "days")
	narrow         string
	long1, longN   string
	tight          bool // short form attaches without a space ("16°C", "16%")
	perSymbol      string
}

var intlUnits = map[string]intlUnitForms{
	"acre": {"ac", "ac", "ac", "acre", "acres", false, "ac"}, "bit": {"bit", "bit", "bit", "bit", "bits", false, "bit"},
	"byte": {"byte", "byte", "B", "byte", "bytes", false, "B"}, "celsius": {"°C", "°C", "°C", "degree Celsius", "degrees Celsius", true, "°C"},
	"centimeter": {"cm", "cm", "cm", "centimeter", "centimeters", false, "cm"}, "day": {"day", "days", "d", "day", "days", false, "d"},
	"degree": {"deg", "deg", "°", "degree", "degrees", false, "deg"}, "fahrenheit": {"°F", "°F", "°", "degree Fahrenheit", "degrees Fahrenheit", true, "°F"},
	"fluid-ounce": {"fl oz", "fl oz", "fl oz", "fluid ounce", "fluid ounces", false, "fl oz"}, "foot": {"ft", "ft", "′", "foot", "feet", false, "ft"},
	"gallon": {"gal", "gal", "gal", "gallon", "gallons", false, "gal"}, "gigabit": {"Gb", "Gb", "Gb", "gigabit", "gigabits", false, "Gb"},
	"gigabyte": {"GB", "GB", "GB", "gigabyte", "gigabytes", false, "GB"}, "gram": {"g", "g", "g", "gram", "grams", false, "g"},
	"hectare": {"ha", "ha", "ha", "hectare", "hectares", false, "ha"}, "hour": {"hr", "hr", "h", "hour", "hours", false, "h"},
	"inch": {"in", "in", "″", "inch", "inches", false, "in"}, "kilobit": {"kb", "kb", "kb", "kilobit", "kilobits", false, "kb"},
	"kilobyte": {"kB", "kB", "kB", "kilobyte", "kilobytes", false, "kB"}, "kilogram": {"kg", "kg", "kg", "kilogram", "kilograms", false, "kg"},
	"kilometer": {"km", "km", "km", "kilometer", "kilometers", false, "km"}, "liter": {"L", "L", "L", "liter", "liters", false, "L"},
	"megabit": {"Mb", "Mb", "Mb", "megabit", "megabits", false, "Mb"}, "megabyte": {"MB", "MB", "MB", "megabyte", "megabytes", false, "MB"},
	"meter": {"m", "m", "m", "meter", "meters", false, "m"}, "microsecond": {"μs", "μs", "μs", "microsecond", "microseconds", false, "μs"},
	"mile": {"mi", "mi", "mi", "mile", "miles", false, "mi"}, "mile-scandinavian": {"smi", "smi", "smi", "mile-scandinavian", "miles-scandinavian", false, "smi"},
	"milliliter": {"mL", "mL", "mL", "milliliter", "milliliters", false, "mL"}, "millimeter": {"mm", "mm", "mm", "millimeter", "millimeters", false, "mm"},
	"millisecond": {"ms", "ms", "ms", "millisecond", "milliseconds", false, "ms"}, "minute": {"min", "min", "m", "minute", "minutes", false, "min"},
	"month": {"mth", "mths", "m", "month", "months", false, "m"}, "nanosecond": {"ns", "ns", "ns", "nanosecond", "nanoseconds", false, "ns"},
	"ounce": {"oz", "oz", "oz", "ounce", "ounces", false, "oz"}, "percent": {"%", "%", "%", "percent", "percent", true, "%"},
	"petabyte": {"PB", "PB", "PB", "petabyte", "petabytes", false, "PB"}, "pound": {"lb", "lb", "lb", "pound", "pounds", false, "lb"},
	"second": {"sec", "sec", "s", "second", "seconds", false, "s"}, "stone": {"st", "st", "st", "stone", "stones", false, "st"},
	"terabit": {"Tb", "Tb", "Tb", "terabit", "terabits", false, "Tb"}, "terabyte": {"TB", "TB", "TB", "terabyte", "terabytes", false, "TB"},
	"week": {"wk", "wks", "w", "week", "weeks", false, "w"}, "yard": {"yd", "yd", "yd", "yard", "yards", false, "yd"},
	"year": {"yr", "yrs", "y", "year", "years", false, "y"},
}

func (nf *intlNumberFormat) unitParts(numParts []intlPart, raw intlRawNumber, x intlDecimal) []intlPart {
	singular := raw.intDigits == "1" && raw.fracDigits == "" && !x.nan && !x.inf
	num, den, isPer := strings.Cut(nf.unit, "-per-")
	if !isPer {
		num = nf.unit
	}
	forms := intlUnits[num]
	var label string
	space := " "
	switch nf.unitDisplay {
	case "long":
		label = forms.longN
		if singular {
			label = forms.long1
		}
		if isPer {
			label += " per " + intlUnits[den].long1
		}
	case "narrow":
		label, space = forms.narrow, ""
		if isPer {
			label = forms.short1 + "/" + intlUnits[den].perSymbol
		}
	default:
		label = forms.shortN
		if singular {
			label = forms.short1
		}
		if forms.tight {
			space = ""
		}
		if isPer {
			label = forms.shortN + "/" + intlUnits[den].perSymbol
			if num == "kilometer" && den == "hour" {
				label = "km/h"
			}
		}
	}
	out := append([]intlPart{}, numParts...)
	if space != "" {
		out = append(out, intlPart{"literal", space})
	}
	return append(out, intlPart{"unit", label})
}

// intlAccountingParens are the languages whose CLDR accounting currency
// pattern wraps negatives in parentheses; the rest keep the minus sign.
var intlAccountingParens = map[string]bool{
	"en": true, "fr": true, "ja": true, "ko": true, "zh": true, "nl": true, "th": true, "hi": true,
	"id": true, "ms": true, "fil": true, "he": true, "ar": true,
}

func intlLanguageOf(locale string) string {
	lang, _, _ := strings.Cut(locale, "-")
	return lang
}

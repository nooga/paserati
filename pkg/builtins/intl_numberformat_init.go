package builtins

import (
	"github.com/nooga/paserati/pkg/vm"
)

// intlToMathematicalValue implements ToIntlMathematicalValue: BigInts and
// numeric strings stay exact, everything else goes through ToNumber.
func intlToMathematicalValue(vmInstance *vm.VM, v vm.Value) (intlDecimal, error) {
	if v.IsObject() || v.IsCallable() {
		vmInstance.EnterHelperCall()
		v = vmInstance.ToPrimitive(v, "number")
		vmInstance.ExitHelperCall()
		if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
			return intlDecimal{}, ErrVMUnwinding
		}
	}
	switch v.Type() {
	case vm.TypeBigInt:
		return intlDecimalFromBigInt(v.AsBigInt()), nil
	case vm.TypeString:
		if d, ok := intlDecimalFromString(v.ToString()); ok {
			return d, nil
		}
		return intlDecimal{nan: true}, nil
	}
	f, err := toNumberWithVM(vmInstance, v)
	if err != nil {
		return intlDecimal{}, err
	}
	return intlDecimalFromFloat(f), nil
}

// intlPartsArray is the formatToParts result: an Array of {type, value}.
func intlPartsArray(vmInstance *vm.VM, parts []intlPart, source []string) vm.Value {
	elems := make([]vm.Value, len(parts))
	for i, p := range parts {
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		obj.SetOwn("type", vm.NewString(p.typ))
		obj.SetOwn("value", vm.NewString(p.value))
		if source != nil {
			obj.SetOwn("source", vm.NewString(source[i]))
		}
		elems[i] = vm.NewValueFromPlainObject(obj)
	}
	return vmInstance.NewArrayFromSlice(elems)
}

// intlNumberRange formats x..y (FormatNumericRangeToParts): an approximate
// "~x" when both render the same, else start, a dash and end. Affixed
// styles (currency, percent, units) get a spaced dash, as ICU does.
func (nf *intlNumberFormat) rangeParts(x, y intlDecimal) ([]intlPart, []string) {
	xp, yp := nf.partsFor(x), nf.partsFor(y)
	if intlJoinParts(xp) == intlJoinParts(yp) {
		parts := append([]intlPart{{"approximatelySign", "~"}}, xp...)
		src := make([]string, len(parts))
		for i := range src {
			src[i] = "shared"
		}
		return parts, src
	}
	dash := "–"
	for _, p := range append(append([]intlPart{}, xp...), yp...) {
		switch p.typ {
		case "currency", "percentSign", "unit", "compact", "literal":
			dash = " – "
		}
	}
	var parts []intlPart
	var src []string
	for _, p := range xp {
		parts, src = append(parts, p), append(src, "startRange")
	}
	parts, src = append(parts, intlPart{"literal", dash}), append(src, "shared")
	for _, p := range yp {
		parts, src = append(parts, p), append(src, "endRange")
	}
	return parts, src
}

func installIntlNumberFormat(vmInstance *vm.VM, intlObj *vm.PlainObject) {
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	vmInstance.IntlNumberFormatPrototype = protoVal

	var ctor vm.Value
	ctor = vm.NewConstructorWithProps(0, false, "NumberFormat", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			newTarget = ctor
		}
		objProto, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%Intl.NumberFormat.prototype%")
		if err != nil {
			return intlAbrupt(err)
		}
		nf, err := intlNewNumberFormat(vmInstance, intlArg(args, 0), intlArg(args, 1))
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(objProto).AsPlainObject()
		obj.SetInternalSlots(nf)
		return vm.NewValueFromPlainObject(obj), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", protoVal)
	ctorProps.SetOwnNonEnumerable("supportedLocalesOf", vm.NewNativeFunction(1, false, "supportedLocalesOf", func(args []vm.Value) (vm.Value, error) {
		return intlSupportedLocalesOf(vmInstance, "Intl.NumberFormat", intlArg(args, 0), intlArg(args, 1))
	}))

	proto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(vmInstance, proto, "Intl.NumberFormat")

	receiver := func(method string) (*intlNumberFormat, error) {
		nf, ok := intlSlotsOf(vmInstance.GetThis()).(*intlNumberFormat)
		if !ok {
			return nil, vmInstance.NewTypeError("Method Intl.NumberFormat.prototype." + method + " called on incompatible receiver")
		}
		return nf, nil
	}

	// get Intl.NumberFormat.prototype.format: a bound format function,
	// created once per instance.
	formatGetter := vm.NewNativeFunction(0, false, "get format", func(args []vm.Value) (vm.Value, error) {
		nf, err := receiver("format")
		if err != nil {
			return vm.Undefined, err
		}
		if !nf.formatBound {
			nf.formatBound = true
			nf.boundFormat = vm.NewNativeFunction(1, false, "", func(args []vm.Value) (vm.Value, error) {
				x, err := intlToMathematicalValue(vmInstance, intlArg(args, 0))
				if err != nil {
					return intlAbrupt(err)
				}
				return vm.NewString(intlJoinParts(nf.partsFor(x))), nil
			})
		}
		return nf.boundFormat, nil
	})
	enumerable, configurable := false, true
	proto.DefineAccessorProperty("format", formatGetter, true, vm.Undefined, false, &enumerable, &configurable)

	proto.SetOwnNonEnumerable("formatToParts", vm.NewNativeFunction(1, false, "formatToParts", func(args []vm.Value) (vm.Value, error) {
		nf, err := receiver("formatToParts")
		if err != nil {
			return vm.Undefined, err
		}
		x, err := intlToMathematicalValue(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		return intlPartsArray(vmInstance, nf.partsFor(x), nil), nil
	}))

	rangeArgs := func(method string, args []vm.Value) (*intlNumberFormat, intlDecimal, intlDecimal, error) {
		nf, err := receiver(method)
		if err != nil {
			return nil, intlDecimal{}, intlDecimal{}, err
		}
		start, end := intlArg(args, 0), intlArg(args, 1)
		if start.Type() == vm.TypeUndefined || end.Type() == vm.TypeUndefined {
			return nil, intlDecimal{}, intlDecimal{}, vmInstance.NewTypeError("start and end are required")
		}
		x, err := intlToMathematicalValue(vmInstance, start)
		if err != nil {
			return nil, intlDecimal{}, intlDecimal{}, err
		}
		y, err := intlToMathematicalValue(vmInstance, end)
		if err != nil {
			return nil, intlDecimal{}, intlDecimal{}, err
		}
		if x.nan || y.nan {
			return nil, intlDecimal{}, intlDecimal{}, vmInstance.NewRangeError("Invalid range: NaN")
		}
		return nf, x, y, nil
	}
	proto.SetOwnNonEnumerable("formatRange", vm.NewNativeFunction(2, false, "formatRange", func(args []vm.Value) (vm.Value, error) {
		nf, x, y, err := rangeArgs("formatRange", args)
		if err != nil {
			return intlAbrupt(err)
		}
		parts, _ := nf.rangeParts(x, y)
		return vm.NewString(intlJoinParts(parts)), nil
	}))
	proto.SetOwnNonEnumerable("formatRangeToParts", vm.NewNativeFunction(2, false, "formatRangeToParts", func(args []vm.Value) (vm.Value, error) {
		nf, x, y, err := rangeArgs("formatRangeToParts", args)
		if err != nil {
			return intlAbrupt(err)
		}
		parts, src := nf.rangeParts(x, y)
		return intlPartsArray(vmInstance, parts, src), nil
	}))

	proto.SetOwnNonEnumerable("resolvedOptions", vm.NewNativeFunction(0, false, "resolvedOptions", func(args []vm.Value) (vm.Value, error) {
		nf, err := receiver("resolvedOptions")
		if err != nil {
			return vm.Undefined, err
		}
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		str := func(k, v string) { obj.SetOwn(k, vm.NewString(v)) }
		num := func(k string, v int) { obj.SetOwn(k, vm.NumberValue(float64(v))) }
		str("locale", nf.locale)
		str("numberingSystem", nf.numberingSystem)
		str("style", nf.style)
		if nf.style == "currency" {
			str("currency", nf.currency)
			str("currencyDisplay", nf.currencyDisplay)
			str("currencySign", nf.currencySign)
		}
		if nf.style == "unit" {
			str("unit", nf.unit)
			str("unitDisplay", nf.unitDisplay)
		}
		num("minimumIntegerDigits", nf.minInt)
		if nf.hasFD {
			num("minimumFractionDigits", nf.minFD)
			num("maximumFractionDigits", nf.maxFD)
		}
		if nf.hasSD {
			num("minimumSignificantDigits", nf.minSD)
			num("maximumSignificantDigits", nf.maxSD)
		}
		if nf.useGrouping == "" {
			obj.SetOwn("useGrouping", vm.BooleanValue(false))
		} else {
			str("useGrouping", nf.useGrouping)
		}
		str("notation", nf.notation)
		if nf.notation == "compact" {
			str("compactDisplay", nf.compactDisplay)
		}
		str("signDisplay", nf.signDisplay)
		num("roundingIncrement", nf.roundingIncr)
		str("roundingMode", nf.roundingMode)
		str("roundingPriority", nf.roundingPrio)
		str("trailingZeroDisplay", nf.trailingZero)
		return vm.NewValueFromPlainObject(obj), nil
	}))

	intlObj.SetOwnNonEnumerable("NumberFormat", ctor)
}

// intlFormatNumberWith is Number/BigInt.prototype.toLocaleString's body:
// a fresh NumberFormat(locales, options) formatting x.
func intlFormatNumberWith(vmInstance *vm.VM, x vm.Value, locales, options vm.Value) (vm.Value, error) {
	nf, err := intlNewNumberFormat(vmInstance, locales, options)
	if err != nil {
		return intlAbrupt(err)
	}
	d, err := intlToMathematicalValue(vmInstance, x)
	if err != nil {
		return intlAbrupt(err)
	}
	return vm.NewString(intlJoinParts(nf.partsFor(d))), nil
}

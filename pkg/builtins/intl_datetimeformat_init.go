package builtins

import (
	"time"

	"github.com/nooga/paserati/pkg/vm"
)

// intlDateValue is the date argument of format/formatToParts: now when
// undefined, else ToNumber, then TimeClip (NaN is a RangeError).
func intlDateValue(vmInstance *vm.VM, v vm.Value) (float64, error) {
	if v.Type() == vm.TypeUndefined {
		return float64(time.Now().UnixMilli()), nil
	}
	if v.IsObject() {
		if ts, ok := v.AsPlainObject().GetInternal("__timestamp__"); ok {
			return intlTimeClip(vmInstance, ts.ToFloat())
		}
	}
	f, err := toNumberWithVM(vmInstance, v)
	if err != nil {
		return 0, err
	}
	return intlTimeClip(vmInstance, f)
}

func installIntlDateTimeFormat(vmInstance *vm.VM, intlObj *vm.PlainObject) {
	proto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	vmInstance.IntlDateTimeFormatPrototype = protoVal

	var ctor vm.Value
	ctor = vm.NewConstructorWithProps(0, false, "DateTimeFormat", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			newTarget = ctor
		}
		objProto, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%Intl.DateTimeFormat.prototype%")
		if err != nil {
			return intlAbrupt(err)
		}
		dtf, err := intlNewDateTimeFormat(vmInstance, intlArg(args, 0), intlArg(args, 1), "any", "date")
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(objProto).AsPlainObject()
		obj.SetInternalSlots(dtf)
		return vm.NewValueFromPlainObject(obj), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", protoVal)
	ctorProps.SetOwnNonEnumerable("supportedLocalesOf", vm.NewNativeFunction(1, false, "supportedLocalesOf", func(args []vm.Value) (vm.Value, error) {
		return intlSupportedLocalesOfWith(vmInstance, "Intl.DateTimeFormat", intlIsEnglishLocale, intlArg(args, 0), intlArg(args, 1))
	}))

	proto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(vmInstance, proto, "Intl.DateTimeFormat")

	receiver := func(method string) (*intlDateTimeFormat, error) {
		dtf, ok := intlSlotsOf(vmInstance.GetThis()).(*intlDateTimeFormat)
		if !ok {
			return nil, vmInstance.NewTypeError("Method Intl.DateTimeFormat.prototype." + method + " called on incompatible receiver")
		}
		return dtf, nil
	}

	formatGetter := vm.NewNativeFunction(0, false, "get format", func(args []vm.Value) (vm.Value, error) {
		dtf, err := receiver("format")
		if err != nil {
			return vm.Undefined, err
		}
		if !dtf.formatBound {
			dtf.formatBound = true
			dtf.boundFormat = vm.NewNativeFunction(1, false, "", func(args []vm.Value) (vm.Value, error) {
				x, err := intlDateValue(vmInstance, intlArg(args, 0))
				if err != nil {
					return intlAbrupt(err)
				}
				return vm.NewString(intlJoinParts(dtf.partsFor(x))), nil
			})
		}
		return dtf.boundFormat, nil
	})
	enumerable, configurable := false, true
	proto.DefineAccessorProperty("format", formatGetter, true, vm.Undefined, false, &enumerable, &configurable)

	proto.SetOwnNonEnumerable("formatToParts", vm.NewNativeFunction(1, false, "formatToParts", func(args []vm.Value) (vm.Value, error) {
		dtf, err := receiver("formatToParts")
		if err != nil {
			return vm.Undefined, err
		}
		x, err := intlDateValue(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		return intlPartsArray(vmInstance, dtf.partsFor(x), nil), nil
	}))

	rangeArgs := func(method string, args []vm.Value) (*intlDateTimeFormat, float64, float64, error) {
		dtf, err := receiver(method)
		if err != nil {
			return nil, 0, 0, err
		}
		start, end := intlArg(args, 0), intlArg(args, 1)
		if start.Type() == vm.TypeUndefined || end.Type() == vm.TypeUndefined {
			return nil, 0, 0, vmInstance.NewTypeError("startDate and endDate are required")
		}
		x, err := intlDateValue(vmInstance, start)
		if err != nil {
			return nil, 0, 0, err
		}
		y, err := intlDateValue(vmInstance, end)
		if err != nil {
			return nil, 0, 0, err
		}
		return dtf, x, y, nil
	}
	proto.SetOwnNonEnumerable("formatRange", vm.NewNativeFunction(2, false, "formatRange", func(args []vm.Value) (vm.Value, error) {
		dtf, x, y, err := rangeArgs("formatRange", args)
		if err != nil {
			return intlAbrupt(err)
		}
		parts, _ := dtf.rangeParts(x, y)
		return vm.NewString(intlJoinParts(parts)), nil
	}))
	proto.SetOwnNonEnumerable("formatRangeToParts", vm.NewNativeFunction(2, false, "formatRangeToParts", func(args []vm.Value) (vm.Value, error) {
		dtf, x, y, err := rangeArgs("formatRangeToParts", args)
		if err != nil {
			return intlAbrupt(err)
		}
		parts, src := dtf.rangeParts(x, y)
		return intlPartsArray(vmInstance, parts, src), nil
	}))

	proto.SetOwnNonEnumerable("resolvedOptions", vm.NewNativeFunction(0, false, "resolvedOptions", func(args []vm.Value) (vm.Value, error) {
		dtf, err := receiver("resolvedOptions")
		if err != nil {
			return vm.Undefined, err
		}
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		str := func(k, v string) {
			if v != "" {
				obj.SetOwn(k, vm.NewString(v))
			}
		}
		str("locale", dtf.locale)
		str("calendar", dtf.calendar)
		str("numberingSystem", dtf.numberingSystem)
		str("timeZone", dtf.timeZone)
		if dtf.hourCycle != "" {
			str("hourCycle", dtf.hourCycle)
			obj.SetOwn("hour12", vm.BooleanValue(dtf.hourCycle == "h11" || dtf.hourCycle == "h12"))
		}
		if dtf.dateStyle == "" && dtf.timeStyle == "" {
			str("weekday", dtf.weekday)
			str("era", dtf.era)
			str("year", dtf.year)
			str("month", dtf.month)
			str("day", dtf.day)
			str("dayPeriod", dtf.dayPeriod)
			hour, minute, second := dtf.hour, dtf.minute, dtf.second
			// The rendered pattern always pads minutes and seconds after an hour.
			if hour != "" && minute != "" {
				minute = "2-digit"
			}
			if minute != "" && second != "" {
				second = "2-digit"
			}
			if dtf.hourCycle == "h23" || dtf.hourCycle == "h24" {
				if hour != "" {
					hour = "2-digit"
				}
			}
			str("hour", hour)
			str("minute", minute)
			str("second", second)
			if dtf.fsd > 0 {
				obj.SetOwn("fractionalSecondDigits", vm.NumberValue(float64(dtf.fsd)))
			}
			str("timeZoneName", dtf.timeZoneName)
		}
		str("dateStyle", dtf.dateStyle)
		str("timeStyle", dtf.timeStyle)
		return vm.NewValueFromPlainObject(obj), nil
	}))

	intlObj.SetOwnNonEnumerable("DateTimeFormat", ctor)
}

// intlSupportedLocalesOfWith is supportedLocalesOf for a constructor whose
// available locales are a subset (those accepted by available).
func intlSupportedLocalesOfWith(vmInstance *vm.VM, owner string, available func(string) bool, locales, options vm.Value) (vm.Value, error) {
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
	var out []string
	for _, locale := range requested {
		if candidate, ok := intlBestAvailableLocale(intlRemoveUnicodeExtensions(locale)); ok && available(candidate) {
			out = append(out, locale)
		}
	}
	return intlStringList(vmInstance, out), nil
}

// intlFormatDateWith is the shared body of Date.prototype.toLocaleString,
// toLocaleDateString and toLocaleTimeString.
func intlFormatDateWith(vmInstance *vm.VM, ms float64, locales, options vm.Value, required, defaults string) (vm.Value, error) {
	dtf, err := intlNewDateTimeFormat(vmInstance, locales, options, required, defaults)
	if err != nil {
		return intlAbrupt(err)
	}
	return vm.NewString(intlJoinParts(dtf.partsFor(ms))), nil
}

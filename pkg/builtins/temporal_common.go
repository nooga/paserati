package builtins

import (
	"math"
	"math/big"
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Shared plumbing for the Temporal bindings. Each Temporal type lives in its
// own temporal_<type>.go file, registers an installer with
// registerTemporalInstaller from an init function, and talks to the other
// types only through the cross-type operations named in temporal_contract.go.
// The engine-independent algorithms are in package temporal.

type temporalInstaller func(r *temporalRealm) error

var temporalInstallers []temporalInstaller

func registerTemporalInstaller(f temporalInstaller) {
	temporalInstallers = append(temporalInstallers, f)
}

// temporalRealm is one VM's Temporal state.
type temporalRealm struct {
	vm     *vm.VM
	ns     *vm.PlainObject            // the Temporal namespace object
	protos map[string]*vm.PlainObject // "Instant" -> %Temporal.Instant.prototype%
	ctors  map[string]vm.Value
}

// Internal slots of each type. The calendar is always iso8601.
type (
	tInstant       struct{ ns *big.Int }
	tPlainDate     struct{ date temporal.Date }
	tPlainTime     struct{ time temporal.Time }
	tPlainDateTime struct{ dt temporal.DateTime }
	// A year-month keeps a reference ISO day, a month-day a reference year.
	tPlainYearMonth struct{ date temporal.Date }
	tPlainMonthDay  struct{ date temporal.Date }
	tDuration       struct{ d temporal.Duration }
	tZoned          struct {
		ns *big.Int
		tz temporal.TimeZone
	}
)

// slotsOf returns v's internal slots if they are a *T (the brand check).
func slotsOf[T any](v vm.Value) (*T, bool) {
	if v.Type() != vm.TypeObject {
		return nil, false
	}
	s, ok := v.AsPlainObject().InternalSlots().(*T)
	return s, ok
}

// thisSlots is the receiver brand check every prototype method starts with.
func thisSlots[T any](r *temporalRealm, typeName string) (*T, error) {
	s, ok := slotsOf[T](r.vm.GetThis())
	if !ok {
		return nil, r.vm.NewTypeError("Method called on incompatible receiver: not a Temporal." + typeName)
	}
	return s, nil
}

// wrap creates an object of the named type around slots.
func (r *temporalRealm) wrap(typeName string, slots any) vm.Value {
	return r.wrapWithProto(vm.NewValueFromPlainObject(r.protos[typeName]), slots)
}

func (r *temporalRealm) wrapWithProto(proto vm.Value, slots any) vm.Value {
	obj := vm.NewObject(proto).AsPlainObject()
	obj.SetInternalSlots(slots)
	return vm.NewValueFromPlainObject(obj)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// err converts an error from package temporal to the JS error the spec
// throws; other errors (JS exceptions in flight) pass through.
func (r *temporalRealm) err(e error) error {
	switch t := e.(type) {
	case nil:
		return nil
	case *temporal.RangeError:
		return r.vm.NewRangeError(t.Msg)
	case *temporal.TypeError:
		return r.vm.NewTypeError(t.Msg)
	}
	return e
}

func (r *temporalRealm) rangeErr(msg string) error { return r.vm.NewRangeError(msg) }
func (r *temporalRealm) typeErr(msg string) error  { return r.vm.NewTypeError(msg) }

// ---------------------------------------------------------------------------
// Class construction
// ---------------------------------------------------------------------------

// protoRef is the prototype for an object being constructed. It is resolved
// from newTarget only when the object is created, after the constructor has
// converted and validated its arguments (OrdinaryCreateFromConstructor comes
// last in CreateTemporalX), because reading newTarget.prototype is
// observable.
type protoRef func() (vm.Value, error)

func staticProto(p *vm.PlainObject) protoRef {
	return func() (vm.Value, error) { return vm.NewValueFromPlainObject(p), nil }
}

// wrapNew creates an object of the constructor's prototype around slots.
func (r *temporalRealm) wrapNew(proto protoRef, slots any) (vm.Value, error) {
	p, err := proto()
	if err != nil {
		return vm.Undefined, err
	}
	return r.wrapWithProto(p, slots), nil
}

// newClass creates Temporal.<name> with its prototype and installs it on the
// namespace. construct receives the arguments and a protoRef to create the
// new object with.
func (r *temporalRealm) newClass(name string, length int, construct func(args []vm.Value, proto protoRef) (vm.Value, error)) (ctor vm.Value, proto *vm.PlainObject) {
	proto = vm.NewObject(r.vm.ObjectPrototype).AsPlainObject()
	protoVal := vm.NewValueFromPlainObject(proto)
	ctor = vm.NewConstructorWithProps(length, false, name, func(args []vm.Value) (vm.Value, error) {
		newTarget := r.vm.GetNewTarget()
		if newTarget.IsUndefined() {
			return vm.Undefined, r.vm.NewTypeError("Constructor Temporal." + name + " requires 'new'")
		}
		return construct(args, func() (vm.Value, error) {
			p, err := r.vm.GetProperty(newTarget, "prototype")
			if err != nil {
				return vm.Undefined, err
			}
			if !p.IsObject() {
				p = protoVal
			}
			return p, nil
		})
	})
	props := ctor.AsNativeFunctionWithProps().Properties
	props.DefineFixedProperty("prototype", protoVal)
	proto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(r.vm, proto, "Temporal."+name)
	r.protos[name] = proto
	r.ctors[name] = ctor
	r.ns.SetOwnNonEnumerable(name, ctor)
	return ctor, proto
}

// method defines a prototype (or namespace) method: writable, configurable,
// not enumerable. fn gets the receiver and the arguments.
func (r *temporalRealm) method(obj *vm.PlainObject, name string, length int, fn func(this vm.Value, args []vm.Value) (vm.Value, error)) {
	obj.SetOwnNonEnumerable(name, vm.NewNativeFunction(length, false, name, func(args []vm.Value) (vm.Value, error) {
		return fn(r.vm.GetThis(), args)
	}))
}

// static defines a static method on a constructor.
func (r *temporalRealm) static(ctor vm.Value, name string, length int, fn func(args []vm.Value) (vm.Value, error)) {
	ctor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable(name, vm.NewNativeFunction(length, false, name, fn))
}

// getter defines an accessor property with only a getter.
func (r *temporalRealm) getter(proto *vm.PlainObject, name string, fn func(this vm.Value) (vm.Value, error)) {
	g := vm.NewNativeFunction(0, false, "get "+name, func(args []vm.Value) (vm.Value, error) {
		return fn(r.vm.GetThis())
	})
	enumerable, configurable := false, true
	proto.DefineAccessorProperty(name, g, true, vm.Undefined, false, &enumerable, &configurable)
}

func argAt(args []vm.Value, i int) vm.Value {
	if i < len(args) {
		return args[i]
	}
	return vm.Undefined
}

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

func (r *temporalRealm) toNumber(v vm.Value) (float64, error) { return toNumberWithVM(r.vm, v) }

func (r *temporalRealm) toString(v vm.Value) (string, error) { return getStringValueWithVM(r.vm, v) }

// toIntegerWithTruncation is ToIntegerWithTruncation: NaN and infinities are
// a RangeError, others truncate toward zero.
func (r *temporalRealm) toIntegerWithTruncation(v vm.Value) (float64, error) {
	n, err := r.toNumber(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, r.rangeErr("number must be finite")
	}
	n = math.Trunc(n)
	if n == 0 {
		n = 0 // -0 -> +0
	}
	return n, nil
}

// toPositiveIntegerWithTruncation additionally requires the result >= 1.
func (r *temporalRealm) toPositiveIntegerWithTruncation(v vm.Value) (float64, error) {
	n, err := r.toIntegerWithTruncation(v)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, r.rangeErr("number must be positive")
	}
	return n, nil
}

// toIntegerIfIntegral is ToIntegerIfIntegral: a non-integral Number is a
// RangeError.
func (r *temporalRealm) toIntegerIfIntegral(v vm.Value) (float64, error) {
	n, err := r.toNumber(v)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
		return 0, r.rangeErr("number must be an integer")
	}
	if n == 0 {
		n = 0
	}
	return n, nil
}

// toBigInt is ToBigInt for the epochNanoseconds arguments: it accepts a
// BigInt, a boolean, or a string that parses as a BigInt, and throws a
// TypeError for numbers and other types.
func (r *temporalRealm) toBigInt(v vm.Value) (*big.Int, error) {
	if v.IsObject() || v.IsCallable() {
		r.vm.EnterHelperCall()
		v = r.vm.ToPrimitive(v, "number")
		r.vm.ExitHelperCall()
		if r.vm.IsUnwinding() || r.vm.IsHandlerFound() {
			return nil, ErrVMUnwinding
		}
	}
	switch v.Type() {
	case vm.TypeBigInt:
		return new(big.Int).Set(v.AsBigInt()), nil
	case vm.TypeBoolean:
		if v.AsBoolean() {
			return big.NewInt(1), nil
		}
		return big.NewInt(0), nil
	case vm.TypeString:
		s := strings.TrimSpace(v.ToString())
		if s == "" {
			return big.NewInt(0), nil
		}
		n, ok := new(big.Int).SetString(s, 0)
		if !ok {
			return nil, r.vm.NewSyntaxError("Cannot convert " + v.ToString() + " to a BigInt")
		}
		return n, nil
	}
	return nil, r.typeErr("Cannot convert value to a BigInt")
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// getOptionsObject is GetOptionsObject: undefined gives Undefined (no
// options), an object is itself, anything else is a TypeError.
func (r *temporalRealm) getOptionsObject(v vm.Value) (vm.Value, error) {
	if v.IsUndefined() {
		return vm.Undefined, nil
	}
	if !isObjectValue(v) {
		return vm.Undefined, r.typeErr("options must be an object or undefined")
	}
	return v, nil
}

// copyOptions is GetOptionsObject plus the spec's CopyDataProperties users
// (none here), returning the object to read options from.
func (r *temporalRealm) option(options vm.Value, name string) (vm.Value, error) {
	if options.IsUndefined() {
		return vm.Undefined, nil
	}
	return r.vm.GetProperty(options, name)
}

// enumOption is GetOption for a string-valued option with a fixed set of
// values. present reports whether the option was given (not undefined).
func (r *temporalRealm) enumOption(options vm.Value, name string, allowed []string, def string) (value string, present bool, err error) {
	v, err := r.option(options, name)
	if err != nil {
		return "", false, err
	}
	if v.IsUndefined() {
		return def, false, nil
	}
	s, err := r.toString(v)
	if err != nil {
		return "", false, err
	}
	for _, a := range allowed {
		if a == s {
			return s, true, nil
		}
	}
	return "", true, r.rangeErr(s + " is not a valid value for option " + name)
}

func (r *temporalRealm) overflowOption(options vm.Value) (temporal.Overflow, error) {
	s, _, err := r.enumOption(options, "overflow", []string{"constrain", "reject"}, "constrain")
	if s == "reject" {
		return temporal.OverflowReject, err
	}
	return temporal.OverflowConstrain, err
}

var roundingModeNames = []string{"ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"}

func (r *temporalRealm) roundingModeOption(options vm.Value, def temporal.RoundingMode) (temporal.RoundingMode, error) {
	s, present, err := r.enumOption(options, "roundingMode", roundingModeNames, "")
	if err != nil || !present {
		return def, err
	}
	for i, n := range roundingModeNames {
		if n == s {
			return temporal.RoundingMode(i), nil
		}
	}
	return def, nil
}

func (r *temporalRealm) disambiguationOption(options vm.Value) (temporal.Disambiguation, error) {
	s, _, err := r.enumOption(options, "disambiguation", []string{"compatible", "earlier", "later", "reject"}, "compatible")
	switch s {
	case "earlier":
		return temporal.DisambiguateEarlier, err
	case "later":
		return temporal.DisambiguateLater, err
	case "reject":
		return temporal.DisambiguateReject, err
	}
	return temporal.DisambiguateCompatible, err
}

// offsetOption reads the "offset" option; def is the caller's default.
func (r *temporalRealm) offsetOption(options vm.Value, def temporal.OffsetOption) (temporal.OffsetOption, error) {
	s, present, err := r.enumOption(options, "offset", []string{"prefer", "use", "ignore", "reject"}, "")
	if err != nil || !present {
		return def, err
	}
	switch s {
	case "use":
		return temporal.OffsetUse, nil
	case "ignore":
		return temporal.OffsetIgnore, nil
	case "reject":
		return temporal.OffsetReject, nil
	}
	return temporal.OffsetPrefer, nil
}

// roundingIncrementOption is GetRoundingIncrementOption: an integer in
// 1..10^9 (truncated), default 1.
func (r *temporalRealm) roundingIncrementOption(options vm.Value) (int64, error) {
	v, err := r.option(options, "roundingIncrement")
	if err != nil || v.IsUndefined() {
		return 1, err
	}
	n, err := r.toIntegerWithTruncation(v)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > 1e9 {
		return 0, r.rangeErr("roundingIncrement out of range")
	}
	return int64(n), nil
}

var unitNames = map[string]temporal.Unit{
	"year": temporal.UnitYear, "years": temporal.UnitYear,
	"month": temporal.UnitMonth, "months": temporal.UnitMonth,
	"week": temporal.UnitWeek, "weeks": temporal.UnitWeek,
	"day": temporal.UnitDay, "days": temporal.UnitDay,
	"hour": temporal.UnitHour, "hours": temporal.UnitHour,
	"minute": temporal.UnitMinute, "minutes": temporal.UnitMinute,
	"second": temporal.UnitSecond, "seconds": temporal.UnitSecond,
	"millisecond": temporal.UnitMillisecond, "milliseconds": temporal.UnitMillisecond,
	"microsecond": temporal.UnitMicrosecond, "microseconds": temporal.UnitMicrosecond,
	"nanosecond": temporal.UnitNanosecond, "nanoseconds": temporal.UnitNanosecond,
}

// unitGroup says which units GetTemporalUnitValuedOption accepts.
type unitGroup int

const (
	unitsDate     unitGroup = iota // year..day
	unitsTime                      // hour..nanosecond
	unitsDateTime                  // year..nanosecond
)

func (g unitGroup) allows(u temporal.Unit) bool {
	switch g {
	case unitsDate:
		return u.IsDateUnit()
	case unitsTime:
		return u.IsTimeUnit()
	}
	return true
}

// unitDefault is the default of a unit option: a unit, "unset" (absent
// stays absent) or "required".
type unitDefault struct {
	unit     temporal.Unit
	required bool
	unset    bool
}

func defaultUnit(u temporal.Unit) unitDefault { return unitDefault{unit: u} }

var (
	unitRequired = unitDefault{required: true}
	unitUnset    = unitDefault{unset: true}
)

// unitOption is GetTemporalUnitValuedOption. The result has ok=false when
// the option is absent and the default is "unset". "auto" is accepted when
// auto is true and returned as UnitAuto.
func (r *temporalRealm) unitOption(options vm.Value, key string, group unitGroup, def unitDefault, auto bool) (u temporal.Unit, ok bool, err error) {
	v, err := r.option(options, key)
	if err != nil {
		return 0, false, err
	}
	if v.IsUndefined() {
		switch {
		case def.required:
			return 0, false, r.rangeErr("the " + key + " option is required")
		case def.unset:
			return 0, false, nil
		}
		return def.unit, true, nil
	}
	s, err := r.toString(v)
	if err != nil {
		return 0, false, err
	}
	if auto && s == "auto" {
		return temporal.UnitAuto, true, nil
	}
	unit, found := unitNames[s]
	if !found || !group.allows(unit) {
		return 0, false, r.rangeErr(s + " is not a valid value for option " + key)
	}
	return unit, true, nil
}

// fractionalSecondDigitsOption is GetTemporalFractionalSecondDigitsOption:
// -1 for "auto" (the default), else 0..9.
func (r *temporalRealm) fractionalSecondDigitsOption(options vm.Value) (int, error) {
	v, err := r.option(options, "fractionalSecondDigits")
	if err != nil || v.IsUndefined() {
		return -1, err
	}
	if v.Type() != vm.TypeFloatNumber && v.Type() != vm.TypeIntegerNumber {
		s, err := r.toString(v)
		if err != nil {
			return 0, err
		}
		if s != "auto" {
			return 0, r.rangeErr(s + " is not a valid value for option fractionalSecondDigits")
		}
		return -1, nil
	}
	n := v.ToFloat()
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, r.rangeErr("fractionalSecondDigits must be finite")
	}
	n = math.Floor(n)
	if n < 0 || n > 9 {
		return 0, r.rangeErr("fractionalSecondDigits out of range")
	}
	return int(n), nil
}

// smallestUnitToPrecision is ToSecondsStringPrecisionRecord: it maps the
// smallestUnit and fractionalSecondDigits options of toString onto a
// precision for temporal.FormatTime (-1 auto, -2 minute, 0..9) and the unit
// and increment to round to. smallestUnit is ok=false when absent.
func secondsStringPrecision(smallest temporal.Unit, present bool, digits int) (precision int, unit temporal.Unit, increment int64) {
	if present {
		switch smallest {
		case temporal.UnitMinute:
			return -2, temporal.UnitMinute, 1
		case temporal.UnitSecond:
			return 0, temporal.UnitSecond, 1
		case temporal.UnitMillisecond:
			return 3, temporal.UnitMillisecond, 1
		case temporal.UnitMicrosecond:
			return 6, temporal.UnitMicrosecond, 1
		}
		return 9, temporal.UnitNanosecond, 1
	}
	switch {
	case digits < 0:
		return -1, temporal.UnitNanosecond, 1
	case digits == 0:
		return 0, temporal.UnitSecond, 1
	case digits <= 3:
		return digits, temporal.UnitMillisecond, pow10(3 - digits)
	case digits <= 6:
		return digits, temporal.UnitMicrosecond, pow10(6 - digits)
	}
	return digits, temporal.UnitNanosecond, pow10(9 - digits)
}

func pow10(n int) int64 {
	p := int64(1)
	for i := 0; i < n; i++ {
		p *= 10
	}
	return p
}

// calendarNameOption is GetTemporalShowCalendarNameOption.
func (r *temporalRealm) calendarNameOption(options vm.Value) (string, error) {
	s, _, err := r.enumOption(options, "calendarName", []string{"auto", "always", "never", "critical"}, "auto")
	return s, err
}

// ---------------------------------------------------------------------------
// Calendars
// ---------------------------------------------------------------------------

// canonicalCalendar returns "iso8601" for any case of that identifier, or a
// RangeError for others (only the ISO calendar is supported).
func (r *temporalRealm) canonicalCalendar(id string) (string, error) {
	if c, ok := temporal.CanonicalizeCalendarIdentifier(id); ok {
		return c, nil
	}
	return "", r.rangeErr("unsupported calendar " + id)
}

// ---------------------------------------------------------------------------
// Property bags
// ---------------------------------------------------------------------------

// tFields is a property bag read by prepareFields. A nil pointer means the
// property was undefined.
type tFields struct {
	Year, Month, Day                     *int
	Hour, Minute, Second                 *int
	Millisecond, Microsecond, Nanosecond *int
	MonthCode                            *string
	Offset                               *string
	TimeZone                             vm.Value // Undefined when absent
}

// Field names, in the alphabetical order the spec reads them.
const (
	fDay         = "day"
	fHour        = "hour"
	fMicrosecond = "microsecond"
	fMillisecond = "millisecond"
	fMinute      = "minute"
	fMonth       = "month"
	fMonthCode   = "monthCode"
	fNanosecond  = "nanosecond"
	fOffset      = "offset"
	fSecond      = "second"
	fTimeZone    = "timeZone"
	fYear        = "year"
)

// prepareFields is PrepareCalendarFields for the ISO calendar: it reads the
// named fields from item in alphabetical order, converting each as the spec
// does. A field in required that is undefined is a TypeError; with
// partial, at least one field must be present instead (the "with" forms).
func (r *temporalRealm) prepareFields(item vm.Value, names []string, required []string, partial bool) (*tFields, error) {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	f := &tFields{TimeZone: vm.Undefined}
	any := false
	for _, name := range sorted {
		v, err := r.vm.GetProperty(item, name)
		if err != nil {
			return nil, err
		}
		if v.IsUndefined() {
			continue
		}
		any = true
		switch name {
		case fMonthCode:
			s, err := r.monthCodeString(v)
			if err != nil {
				return nil, err
			}
			f.MonthCode = &s
		case fOffset:
			s, err := r.offsetFieldString(v)
			if err != nil {
				return nil, err
			}
			f.Offset = &s
		case fTimeZone:
			f.TimeZone = v
		case fMonth, fDay:
			n, err := r.toPositiveIntegerWithTruncation(v)
			if err != nil {
				return nil, err
			}
			i := int(n)
			if name == fMonth {
				f.Month = &i
			} else {
				f.Day = &i
			}
		default:
			n, err := r.toIntegerWithTruncation(v)
			if err != nil {
				return nil, err
			}
			i := clampToInt(n)
			switch name {
			case fYear:
				f.Year = &i
			case fHour:
				f.Hour = &i
			case fMinute:
				f.Minute = &i
			case fSecond:
				f.Second = &i
			case fMillisecond:
				f.Millisecond = &i
			case fMicrosecond:
				f.Microsecond = &i
			case fNanosecond:
				f.Nanosecond = &i
			}
		}
	}
	if partial {
		if !any {
			return nil, r.typeErr("the object has no Temporal fields")
		}
		return f, nil
	}
	for _, name := range required {
		if f.has(name) {
			continue
		}
		return nil, r.typeErr("missing required property " + name)
	}
	return f, nil
}

func (f *tFields) has(name string) bool {
	switch name {
	case fYear:
		return f.Year != nil
	case fMonth:
		return f.Month != nil
	case fMonthCode:
		return f.MonthCode != nil
	case fDay:
		return f.Day != nil
	case fHour:
		return f.Hour != nil
	case fMinute:
		return f.Minute != nil
	case fSecond:
		return f.Second != nil
	case fMillisecond:
		return f.Millisecond != nil
	case fMicrosecond:
		return f.Microsecond != nil
	case fNanosecond:
		return f.Nanosecond != nil
	case fOffset:
		return f.Offset != nil
	case fTimeZone:
		return !f.TimeZone.IsUndefined()
	}
	return false
}

// clampToInt keeps absurd magnitudes representable; they are range-checked
// (and rejected or constrained) by the callers.
func clampToInt(n float64) int {
	if n > 1e15 {
		return int(1e15)
	}
	if n < -1e15 {
		return int(-1e15)
	}
	return int(n)
}

// monthCodeString is ToMonthCode: ToPrimitive, a String required, plus the
// syntax check ("M01".."M99", optionally a trailing "L").
func (r *temporalRealm) monthCodeString(v vm.Value) (string, error) {
	if isObjectValue(v) {
		r.vm.EnterHelperCall()
		v = r.vm.ToPrimitive(v, "string")
		r.vm.ExitHelperCall()
		if r.vm.IsUnwinding() || r.vm.IsHandlerFound() {
			return "", ErrVMUnwinding
		}
	}
	if v.Type() != vm.TypeString {
		return "", r.typeErr("monthCode must be a string")
	}
	s := v.ToString()
	if !validMonthCodeSyntax(s) {
		return "", r.rangeErr("invalid monthCode " + s)
	}
	return s, nil
}

func validMonthCodeSyntax(s string) bool {
	if len(s) != 3 && len(s) != 4 {
		return false
	}
	if s[0] != 'M' || s[1] < '0' || s[1] > '9' || s[2] < '0' || s[2] > '9' {
		return false
	}
	if len(s) == 4 && s[3] != 'L' {
		return false
	}
	return !(s[1] == '0' && s[2] == '0' && len(s) == 3) || true
}

func (r *temporalRealm) offsetFieldString(v vm.Value) (string, error) {
	if !(v.Type() == vm.TypeString || isObjectValue(v)) {
		return "", r.typeErr("offset must be a string")
	}
	s, err := r.toString(v)
	if err != nil {
		return "", err
	}
	if _, _, perr := temporal.ParseDateTimeUTCOffset(s); perr != nil {
		return "", r.err(perr)
	}
	return s, nil
}

// monthFromFields resolves month and monthCode for the ISO calendar
// (CalendarResolveFields' month handling): both present must agree, a
// leap-month code is a RangeError. ok=false when neither is present.
func (r *temporalRealm) monthFromFields(f *tFields) (month int, ok bool, err error) {
	if f.MonthCode == nil {
		if f.Month == nil {
			return 0, false, nil
		}
		return *f.Month, true, nil
	}
	code := *f.MonthCode
	if len(code) == 4 || code == "M00" || code[1:3] > "12" {
		return 0, false, r.rangeErr("invalid monthCode " + code)
	}
	m := int(code[1]-'0')*10 + int(code[2]-'0')
	if f.Month != nil && *f.Month != m {
		return 0, false, r.rangeErr("month and monthCode do not match")
	}
	return m, true, nil
}

// ---------------------------------------------------------------------------
// Durations (shared by every type's add/subtract/until/since)
// ---------------------------------------------------------------------------

var durationFieldNames = []string{"days", "hours", "microseconds", "milliseconds", "minutes", "months", "nanoseconds", "seconds", "weeks", "years"}

// durationRecordFromObject is ToTemporalPartialDurationRecord followed by
// the validity check (ToTemporalDuration for an object).
func (r *temporalRealm) durationFromObject(item vm.Value) (temporal.Duration, error) {
	var vals [10]float64 // days hours µs ms min months ns sec weeks years
	any := false
	for i, name := range durationFieldNames {
		v, err := r.vm.GetProperty(item, name)
		if err != nil {
			return temporal.Duration{}, err
		}
		if v.IsUndefined() {
			continue
		}
		any = true
		if vals[i], err = r.toIntegerIfIntegral(v); err != nil {
			return temporal.Duration{}, err
		}
	}
	if !any {
		return temporal.Duration{}, r.typeErr("the object has no Duration fields")
	}
	d := temporal.Duration{Days: vals[0], Hours: vals[1], Microseconds: vals[2], Milliseconds: vals[3],
		Minutes: vals[4], Months: vals[5], Nanoseconds: vals[6], Seconds: vals[7], Weeks: vals[8], Years: vals[9]}
	if !temporal.IsValidDuration(d) {
		return temporal.Duration{}, r.rangeErr("invalid duration")
	}
	return d, nil
}

// toTemporalDuration is ToTemporalDuration.
func (r *temporalRealm) toTemporalDuration(item vm.Value) (temporal.Duration, error) {
	if s, ok := slotsOf[tDuration](item); ok {
		return s.d, nil
	}
	if isObjectValue(item) {
		return r.durationFromObject(item)
	}
	if item.Type() != vm.TypeString {
		return temporal.Duration{}, r.typeErr("a duration must be an object or a string")
	}
	d, err := temporal.ParseDurationString(item.ToString())
	if err != nil {
		return temporal.Duration{}, r.err(err)
	}
	if !temporal.IsValidDuration(d) {
		return temporal.Duration{}, r.rangeErr("invalid duration")
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Unit options, read and validated in the spec's order
// ---------------------------------------------------------------------------

// rawUnitOption reads a unit-valued option without validating it: the spec
// reads every option first and validates afterwards, which is observable.
func (r *temporalRealm) rawUnitOption(options vm.Value, key string) (s string, present bool, err error) {
	v, err := r.option(options, key)
	if err != nil || v.IsUndefined() {
		return "", false, err
	}
	s, err = r.toString(v)
	return s, err == nil, err
}

// validateUnit is ValidateTemporalUnitValue: it maps a raw unit name to a
// unit of the group, with "auto" allowed if auto is set.
func (r *temporalRealm) validateUnit(key, s string, present bool, group unitGroup, auto bool) (temporal.Unit, bool, error) {
	if !present {
		return 0, false, nil
	}
	if auto && s == "auto" {
		return temporal.UnitAuto, true, nil
	}
	u, ok := unitNames[s]
	if !ok || !group.allows(u) {
		return 0, false, r.rangeErr(s + " is not a valid value for option " + key)
	}
	return u, true, nil
}

func negateRoundingMode(m temporal.RoundingMode) temporal.RoundingMode {
	switch m {
	case temporal.RoundCeil:
		return temporal.RoundFloor
	case temporal.RoundFloor:
		return temporal.RoundCeil
	case temporal.RoundHalfCeil:
		return temporal.RoundHalfFloor
	case temporal.RoundHalfFloor:
		return temporal.RoundHalfCeil
	}
	return m
}

// differenceSettings is GetDifferenceSettings. since reports the "since"
// operation (which negates the rounding mode).
type differenceSettings struct {
	largest, smallest temporal.Unit
	increment         int64
	mode              temporal.RoundingMode
}

func (r *temporalRealm) differenceSettings(since bool, options vm.Value, group unitGroup, disallowed []temporal.Unit, fallbackSmallest, smallestLargestDefault temporal.Unit) (differenceSettings, error) {
	var s differenceSettings
	largestRaw, largestPresent, err := r.rawUnitOption(options, "largestUnit")
	if err != nil {
		return s, err
	}
	if s.increment, err = r.roundingIncrementOption(options); err != nil {
		return s, err
	}
	if s.mode, err = r.roundingModeOption(options, temporal.RoundTrunc); err != nil {
		return s, err
	}
	smallestRaw, smallestPresent, err := r.rawUnitOption(options, "smallestUnit")
	if err != nil {
		return s, err
	}
	largest, ok, err := r.validateUnit("largestUnit", largestRaw, largestPresent, group, true)
	if err != nil {
		return s, err
	}
	if !ok {
		largest = temporal.UnitAuto
	}
	contains := func(u temporal.Unit) bool {
		for _, d := range disallowed {
			if d == u {
				return true
			}
		}
		return false
	}
	if contains(largest) {
		return s, r.rangeErr("largestUnit " + largest.String() + " is not allowed here")
	}
	if since {
		s.mode = negateRoundingMode(s.mode)
	}
	smallest, ok, err := r.validateUnit("smallestUnit", smallestRaw, smallestPresent, group, false)
	if err != nil {
		return s, err
	}
	if !ok {
		smallest = fallbackSmallest
	}
	if contains(smallest) {
		return s, r.rangeErr("smallestUnit " + smallest.String() + " is not allowed here")
	}
	defaultLargest := temporal.LargerOfTwoUnits(smallestLargestDefault, smallest)
	if largest == temporal.UnitAuto {
		largest = defaultLargest
	}
	if temporal.LargerOfTwoUnits(largest, smallest) != largest {
		return s, r.rangeErr("largestUnit must be larger than smallestUnit")
	}
	if max, ok := temporal.MaximumTemporalDurationRoundingIncrement(smallest); ok {
		if err := temporal.ValidateRoundingIncrement(s.increment, max, false); err != nil {
			return s, r.err(err)
		}
	}
	s.largest, s.smallest = largest, smallest
	return s, nil
}

// ---------------------------------------------------------------------------
// Calendars: the ISO 8601 calendar is the only one, so these only validate
// ---------------------------------------------------------------------------

// hasCalendarSlot reports whether v is a Temporal object that carries a
// calendar (everything except Instant, Duration and PlainTime).
func hasCalendarSlot(v vm.Value) bool {
	if _, ok := slotsOf[tZoned](v); ok {
		return true
	}
	if _, ok := slotsOf[tPlainDate](v); ok {
		return true
	}
	if _, ok := slotsOf[tPlainDateTime](v); ok {
		return true
	}
	if _, ok := slotsOf[tPlainYearMonth](v); ok {
		return true
	}
	_, ok := slotsOf[tPlainMonthDay](v)
	return ok
}

// toCalendarIdentifier is ToTemporalCalendarIdentifier: a Temporal object
// carries the ISO calendar, a string is an identifier or any ISO string whose
// calendar annotation counts (ParseTemporalCalendarString), anything else is
// a TypeError.
func (r *temporalRealm) toCalendarIdentifier(v vm.Value) (string, error) {
	if hasCalendarSlot(v) {
		return "iso8601", nil
	}
	if v.Type() != vm.TypeString {
		return "", r.typeErr("calendar must be a string")
	}
	return r.calendarFromString(v.ToString())
}

func (r *temporalRealm) calendarFromString(s string) (string, error) {
	if c, ok := temporal.CanonicalizeCalendarIdentifier(s); ok {
		return c, nil
	}
	for _, parse := range []func(string) (*temporal.Parsed, error){
		temporal.ParseZonedDateTimeString, temporal.ParsePlainDateTimeString, temporal.ParseInstantString,
		temporal.ParsePlainMonthDayString, temporal.ParsePlainYearMonthString, temporal.ParsePlainTimeString,
	} {
		if p, err := parse(s); err == nil {
			if p.Calendar == "" {
				return "iso8601", nil
			}
			return r.canonicalCalendar(p.Calendar)
		}
	}
	return "", r.rangeErr("invalid calendar " + s)
}

// checkCalendar converts a calendar value and discards the result: with a
// single calendar, only validity matters.
func (r *temporalRealm) checkCalendar(v vm.Value) error {
	_, err := r.toCalendarIdentifier(v)
	return err
}

// checkParsedCalendar validates the calendar annotation of a parsed string.
func (r *temporalRealm) checkParsedCalendar(p *temporal.Parsed) error {
	if p.Calendar == "" {
		return nil
	}
	_, err := r.canonicalCalendar(p.Calendar)
	return err
}

// calendarOfItem is GetTemporalCalendarIdentifierWithISODefault for a
// property bag: a Temporal object is ISO, otherwise its calendar property, if
// any, is validated.
func (r *temporalRealm) calendarOfItem(item vm.Value) error {
	if hasCalendarSlot(item) {
		return nil
	}
	c, err := r.vm.GetProperty(item, "calendar")
	if err != nil || c.IsUndefined() {
		return err
	}
	return r.checkCalendar(c)
}

// calendarAnnotation is FormatCalendarAnnotation for the ISO calendar.
func calendarAnnotation(show string) string {
	switch show {
	case "always":
		return "[u-ca=iso8601]"
	case "critical":
		return "[!u-ca=iso8601]"
	}
	return ""
}

package builtins

import "github.com/nooga/paserati/pkg/types"

// Static (type checker) declarations for the Temporal global. Methods that
// return another Temporal object are typed `any`: a cyclic object type
// (PlainDate.add returning PlainDate) would make structural checks recurse
// forever, so the checker only knows the shapes of what primitives it can
// return.

const PriorityTemporal = 105 // After Date

type TemporalInitializer struct{}

func (t *TemporalInitializer) Name() string { return "Temporal" }

func (t *TemporalInitializer) Priority() int { return PriorityTemporal }

// tMethod declares a method: `required` leading parameters, `total` in all
// (every parameter is `any`).
type tMethod struct {
	name            string
	ret             types.Type
	required, total int
}

type tClassTypes struct {
	getters map[string]types.Type
	methods []tMethod
	ctor    []bool // optionality of each constructor parameter; nil for none
	ctorTys []types.Type
	statics []tMethod
}

func (c tClassTypes) build() (proto, ctor *types.ObjectType) {
	proto = types.NewObjectType()
	for name, ty := range c.getters {
		proto.WithProperty(name, ty)
	}
	for _, m := range c.methods {
		proto.WithProperty(m.name, m.fn())
	}
	ctor = types.NewObjectType()
	if c.ctorTys != nil {
		ctor.WithConstructSignature(&types.Signature{ParameterTypes: c.ctorTys, ReturnType: proto, OptionalParams: c.ctor})
	}
	for _, m := range c.statics {
		ctor.WithProperty(m.name, m.fn())
	}
	ctor.WithProperty("prototype", proto)
	return proto, ctor
}

func (m tMethod) fn() *types.ObjectType {
	params := make([]types.Type, m.total)
	optional := make([]bool, m.total)
	for i := range params {
		params[i] = types.Any
		optional[i] = i >= m.required
	}
	return types.NewOptionalFunction(params, m.ret, optional)
}

func tOptional(n, total int) ([]types.Type, []bool) {
	tys := make([]types.Type, total)
	opt := make([]bool, total)
	for i := range tys {
		tys[i] = types.Number
		opt[i] = i >= n
	}
	return tys, opt
}

func (t *TemporalInitializer) InitTypes(ctx *TypeContext) error {
	num, str, boolean, anyT, undef := types.Number, types.String, types.Boolean, types.Any, types.Undefined
	getters := func(pairs ...any) map[string]types.Type {
		m := map[string]types.Type{}
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i].(string)] = pairs[i+1].(types.Type)
		}
		return m
	}
	toStr := []tMethod{{"toString", str, 0, 1}, {"toJSON", str, 0, 0}, {"toLocaleString", str, 0, 2}, {"valueOf", anyT, 0, 0}}
	withMethods := func(ms ...tMethod) []tMethod { return append(append([]tMethod{}, ms...), toStr...) }
	dateGetters := []any{"calendarId", str, "era", anyT, "eraYear", anyT, "year", num, "month", num, "monthCode", str, "day", num,
		"dayOfWeek", num, "dayOfYear", num, "weekOfYear", anyT, "yearOfWeek", anyT, "daysInWeek", num, "daysInMonth", num,
		"daysInYear", num, "monthsInYear", num, "inLeapYear", boolean}
	timeGetters := []any{"hour", num, "minute", num, "second", num, "millisecond", num, "microsecond", num, "nanosecond", num}
	cmp := tMethod{"compare", num, 2, 2}
	from := tMethod{"from", anyT, 1, 2}
	diff := []tMethod{{"until", anyT, 1, 2}, {"since", anyT, 1, 2}}
	addSub := []tMethod{{"add", anyT, 1, 2}, {"subtract", anyT, 1, 2}}

	classes := map[string]tClassTypes{
		"Instant": {
			getters: getters("epochMilliseconds", num, "epochNanoseconds", types.BigInt),
			methods: withMethods(append(append(append([]tMethod{}, addSub[:2]...), diff...),
				tMethod{"round", anyT, 1, 1}, tMethod{"equals", boolean, 1, 1}, tMethod{"toZonedDateTimeISO", anyT, 1, 1})...),
			ctorTys: []types.Type{types.BigInt}, ctor: []bool{false},
			statics: []tMethod{from, {"fromEpochMilliseconds", anyT, 1, 1}, {"fromEpochNanoseconds", anyT, 1, 1}, cmp},
		},
		"Duration": {
			getters: getters("years", num, "months", num, "weeks", num, "days", num, "hours", num, "minutes", num, "seconds", num,
				"milliseconds", num, "microseconds", num, "nanoseconds", num, "sign", num, "blank", boolean),
			methods: withMethods(tMethod{"with", anyT, 1, 1}, tMethod{"negated", anyT, 0, 0}, tMethod{"abs", anyT, 0, 0},
				tMethod{"add", anyT, 1, 1}, tMethod{"subtract", anyT, 1, 1}, tMethod{"round", anyT, 1, 1}, tMethod{"total", num, 1, 1}),
			statics: []tMethod{from, {"compare", num, 2, 3}},
		},
		"PlainDate": {
			getters: getters(dateGetters...),
			methods: withMethods(append(append([]tMethod{{"with", anyT, 1, 2}, {"withCalendar", anyT, 1, 1}, {"equals", boolean, 1, 1},
				{"toPlainDateTime", anyT, 0, 1}, {"toZonedDateTime", anyT, 1, 1}, {"toPlainYearMonth", anyT, 0, 0}, {"toPlainMonthDay", anyT, 0, 0}},
				addSub...), diff...)...),
			ctorTys: []types.Type{num, num, num, anyT}, ctor: []bool{false, false, false, true},
			statics: []tMethod{from, cmp},
		},
		"PlainTime": {
			getters: getters(timeGetters...),
			methods: withMethods(append(append([]tMethod{{"with", anyT, 1, 2}, {"round", anyT, 1, 1}, {"equals", boolean, 1, 1}},
				addSub[:2]...), diff...)...),
			statics: []tMethod{from, cmp},
		},
		"PlainDateTime": {
			getters: getters(append(append([]any{}, dateGetters...), timeGetters...)...),
			methods: withMethods(append(append([]tMethod{{"with", anyT, 1, 2}, {"withPlainTime", anyT, 0, 1}, {"withCalendar", anyT, 1, 1},
				{"round", anyT, 1, 1}, {"equals", boolean, 1, 1}, {"toZonedDateTime", anyT, 1, 2}, {"toPlainDate", anyT, 0, 0}, {"toPlainTime", anyT, 0, 0}},
				addSub...), diff...)...),
			statics: []tMethod{from, cmp},
		},
		"PlainYearMonth": {
			getters: getters("calendarId", str, "era", anyT, "eraYear", anyT, "year", num, "month", num, "monthCode", str,
				"daysInMonth", num, "daysInYear", num, "monthsInYear", num, "inLeapYear", boolean),
			methods: withMethods(append(append([]tMethod{{"with", anyT, 1, 2}, {"equals", boolean, 1, 1}, {"toPlainDate", anyT, 1, 1}},
				addSub...), diff...)...),
			statics: []tMethod{from, cmp},
		},
		"PlainMonthDay": {
			getters: getters("calendarId", str, "monthCode", str, "day", num),
			methods: withMethods(tMethod{"with", anyT, 1, 2}, tMethod{"equals", boolean, 1, 1}, tMethod{"toPlainDate", anyT, 1, 1}),
			statics: []tMethod{from},
		},
		"ZonedDateTime": {
			getters: getters(append(append([]any{"timeZoneId", str, "epochMilliseconds", num, "epochNanoseconds", types.BigInt,
				"hoursInDay", num, "offsetNanoseconds", num, "offset", str}, dateGetters...), timeGetters...)...),
			methods: withMethods(append(append([]tMethod{{"with", anyT, 1, 2}, {"withPlainTime", anyT, 0, 1}, {"withTimeZone", anyT, 1, 1},
				{"withCalendar", anyT, 1, 1}, {"round", anyT, 1, 1}, {"equals", boolean, 1, 1}, {"startOfDay", anyT, 0, 0},
				{"getTimeZoneTransition", anyT, 1, 1}, {"toInstant", anyT, 0, 0}, {"toPlainDate", anyT, 0, 0}, {"toPlainTime", anyT, 0, 0},
				{"toPlainDateTime", anyT, 0, 0}}, addSub...), diff...)...),
			statics: []tMethod{from, cmp},
		},
	}
	// Constructors with trailing optional numeric parameters.
	for name, n := range map[string][2]int{"PlainTime": {0, 6}, "PlainDateTime": {3, 9}, "PlainYearMonth": {2, 2}, "PlainMonthDay": {2, 2}, "Duration": {0, 10}} {
		c := classes[name]
		c.ctorTys, c.ctor = tOptional(n[0], n[1])
		classes[name] = c
	}
	c := classes["PlainDateTime"]
	c.ctorTys = append(c.ctorTys, anyT)
	c.ctor = append(c.ctor, true)
	classes["PlainDateTime"] = c
	for _, name := range []string{"PlainYearMonth", "PlainMonthDay"} {
		c := classes[name]
		c.ctorTys = append(c.ctorTys, anyT, num)
		c.ctor = append(c.ctor, true, true)
		classes[name] = c
	}
	zc := classes["ZonedDateTime"]
	zc.ctorTys, zc.ctor = []types.Type{types.BigInt, str, anyT}, []bool{false, false, true}
	classes["ZonedDateTime"] = zc

	temporalType := types.NewObjectType()
	for name, cls := range classes {
		_, ctor := cls.build()
		temporalType.WithProperty(name, ctor)
	}
	now := types.NewObjectType()
	for _, m := range []tMethod{{"timeZoneId", str, 0, 0}, {"instant", anyT, 0, 0}, {"plainDateISO", anyT, 0, 1},
		{"plainTimeISO", anyT, 0, 1}, {"plainDateTimeISO", anyT, 0, 1}, {"zonedDateTimeISO", anyT, 0, 1}} {
		now.WithProperty(m.name, m.fn())
	}
	temporalType.WithProperty("Now", now)
	_ = undef
	return ctx.DefineGlobal("Temporal", temporalType)
}

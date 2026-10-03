package builtins

import (
	"fmt"
	"math/big"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

var dateTimeFieldNames = []string{fDay, fHour, fMicrosecond, fMillisecond, fMinute, fMonth, fMonthCode, fNanosecond, fSecond, fYear}

func init() {
	registerTemporalInstaller(installPlainDateTime)
}

func num(n int) vm.Value { return vm.NumberValue(float64(n)) }

func isSlots[T any](v vm.Value) bool { _, ok := slotsOf[T](v); return ok }

func (r *temporalRealm) createPlainDateTime(dt temporal.DateTime) (vm.Value, error) {
	if !temporal.ISODateTimeWithinLimits(dt) {
		return vm.Undefined, r.rangeErr("date-time is outside the supported range")
	}
	return r.wrap("PlainDateTime", &tPlainDateTime{dt}), nil
}

// calendarFromValue is ToTemporalCalendarIdentifier: Temporal objects carry
// the ISO calendar, anything else must be a string.
func (r *temporalRealm) calendarFromValue(v vm.Value) error {
	if isObjectValue(v) && (isSlots[tPlainDate](v) || isSlots[tPlainDateTime](v) || isSlots[tPlainMonthDay](v) ||
		isSlots[tPlainYearMonth](v) || isSlots[tZoned](v)) {
		return nil
	}
	if v.Type() != vm.TypeString {
		return r.typeErr("calendar must be a string")
	}
	return r.calendarFromString(v.ToString())
}

// calendarFromString is ParseTemporalCalendarString: an identifier, or any
// ISO date/time string with an optional calendar annotation.
func (r *temporalRealm) calendarFromString(s string) error {
	if _, err := r.canonicalCalendar(s); err == nil {
		return nil
	}
	var p *temporal.Parsed
	var err error
	for _, parse := range []func(string) (*temporal.Parsed, error){temporal.ParsePlainDateTimeString,
		temporal.ParsePlainTimeString, temporal.ParsePlainYearMonthString, temporal.ParsePlainMonthDayString} {
		if p, err = parse(s); err == nil {
			break
		}
	}
	if err != nil {
		return r.rangeErr("invalid calendar " + s)
	}
	if p.Calendar != "" {
		_, err = r.canonicalCalendar(p.Calendar)
		return err
	}
	return nil
}

// toTemporalDateTime is ToTemporalDateTime.
func (r *temporalRealm) toTemporalDateTime(item, options vm.Value) (temporal.DateTime, error) {
	if isObjectValue(item) {
		var dt temporal.DateTime
		known := true
		if s, ok := slotsOf[tPlainDateTime](item); ok {
			dt = s.dt
		} else if s, ok := slotsOf[tPlainDate](item); ok {
			dt = temporal.DateTime{Date: s.date}
		} else if s, ok := slotsOf[tZoned](item); ok {
			dt = s.tz.LocalDateTime(s.ns)
		} else {
			known = false
		}
		if !known {
			return r.dateTimeFromBag(item, options)
		}
		opts, err := r.getOptionsObject(options)
		if err != nil {
			return dt, err
		}
		_, err = r.overflowOption(opts)
		return dt, err
	}
	if item.Type() != vm.TypeString {
		return temporal.DateTime{}, r.typeErr("cannot convert value to a PlainDateTime")
	}
	p, err := temporal.ParsePlainDateTimeString(item.ToString())
	if err != nil {
		return temporal.DateTime{}, r.err(err)
	}
	if p.Calendar != "" {
		if _, err := r.canonicalCalendar(p.Calendar); err != nil {
			return temporal.DateTime{}, err
		}
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return temporal.DateTime{}, err
	}
	if _, err := r.overflowOption(opts); err != nil {
		return temporal.DateTime{}, err
	}
	dt := temporal.DateTime{Date: p.Date, Time: p.Time}
	if !temporal.ISODateTimeWithinLimits(dt) {
		return dt, r.rangeErr("date-time is outside the supported range")
	}
	return dt, nil
}

func (r *temporalRealm) dateTimeFromBag(item, options vm.Value) (temporal.DateTime, error) {
	cal, err := r.vm.GetProperty(item, "calendar")
	if err != nil {
		return temporal.DateTime{}, err
	}
	if !cal.IsUndefined() {
		if err := r.calendarFromValue(cal); err != nil {
			return temporal.DateTime{}, err
		}
	}
	f, err := r.prepareFields(item, dateTimeFieldNames, []string{fYear, fDay}, false)
	if err != nil {
		return temporal.DateTime{}, err
	}
	if f.Month == nil && f.MonthCode == nil {
		return temporal.DateTime{}, r.typeErr("missing required property month or monthCode")
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return temporal.DateTime{}, err
	}
	overflow, err := r.overflowOption(opts)
	if err != nil {
		return temporal.DateTime{}, err
	}
	month, _, err := r.monthFromFields(f)
	if err != nil {
		return temporal.DateTime{}, err
	}
	return r.regulateDateTime(*f.Year, month, *f.Day, f, overflow)
}

func orZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func (r *temporalRealm) regulateDateTime(y, m, d int, f *tFields, overflow temporal.Overflow) (temporal.DateTime, error) {
	date, err := temporal.RegulateISODate(y, m, d, overflow)
	if err != nil {
		return temporal.DateTime{}, r.err(err)
	}
	t, err := temporal.RegulateTime(orZero(f.Hour), orZero(f.Minute), orZero(f.Second), orZero(f.Millisecond), orZero(f.Microsecond), orZero(f.Nanosecond), overflow)
	if err != nil {
		return temporal.DateTime{}, r.err(err)
	}
	dt := temporal.DateTime{Date: date, Time: t}
	if !temporal.ISODateTimeWithinLimits(dt) {
		return dt, r.rangeErr("date-time is outside the supported range")
	}
	return dt, nil
}

func compareDateTime(a, b temporal.DateTime) int {
	if c := temporal.CompareISODate(a.Date, b.Date); c != 0 {
		return c
	}
	return temporal.CompareTime(a.Time, b.Time)
}

// rejectObjectWithCalendarOrTimeZone is RejectObjectWithCalendarOrTimeZone.
func (r *temporalRealm) rejectObjectWithCalendarOrTimeZone(v vm.Value) error {
	if !isObjectValue(v) {
		return r.typeErr("argument must be an object")
	}
	if isSlots[tPlainDate](v) || isSlots[tPlainDateTime](v) || isSlots[tPlainMonthDay](v) ||
		isSlots[tPlainTime](v) || isSlots[tPlainYearMonth](v) || isSlots[tZoned](v) {
		return r.typeErr("argument must be a plain object, not a Temporal object")
	}
	for _, k := range []string{"calendar", "timeZone"} {
		p, err := r.vm.GetProperty(v, k)
		if err != nil {
			return err
		}
		if !p.IsUndefined() {
			return r.typeErr("argument must not have a " + k + " property")
		}
	}
	return nil
}

func calendarAnnotation(mode string) string {
	switch mode {
	case "always":
		return "[u-ca=iso8601]"
	case "critical":
		return "[!u-ca=iso8601]"
	}
	return ""
}

func installPlainDateTime(r *temporalRealm) error {
	ctor, proto := r.newClass("PlainDateTime", 3, func(args []vm.Value, p vm.Value) (vm.Value, error) {
		var n [9]int
		for i := range n {
			a := argAt(args, i)
			if i >= 3 && a.IsUndefined() {
				continue
			}
			f, err := r.toIntegerWithTruncation(a)
			if err != nil {
				return vm.Undefined, err
			}
			n[i] = clampToInt(f)
		}
		if cal := argAt(args, 9); !cal.IsUndefined() {
			if cal.Type() != vm.TypeString {
				return vm.Undefined, r.typeErr("calendar must be a string")
			}
			if _, err := r.canonicalCalendar(cal.ToString()); err != nil {
				return vm.Undefined, err
			}
		}
		if !temporal.IsValidISODate(n[0], n[1], n[2]) || !temporal.IsValidTime(n[3], n[4], n[5], n[6], n[7], n[8]) {
			return vm.Undefined, r.rangeErr("invalid date-time")
		}
		dt := temporal.DateTime{Date: temporal.Date{Year: n[0], Month: n[1], Day: n[2]},
			Time: temporal.Time{Hour: n[3], Minute: n[4], Second: n[5], Millisecond: n[6], Microsecond: n[7], Nanosecond: n[8]}}
		if !temporal.ISODateTimeWithinLimits(dt) {
			return vm.Undefined, r.rangeErr("date-time is outside the supported range")
		}
		return r.wrapWithProto(p, &tPlainDateTime{dt}), nil
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		dt, err := r.toTemporalDateTime(argAt(args, 0), argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDateTime(dt)
	})
	r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
		a, err := r.toTemporalDateTime(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		b, err := r.toTemporalDateTime(argAt(args, 1), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return num(compareDateTime(a, b)), nil
	})

	get := func(name string, f func(dt temporal.DateTime) vm.Value) {
		r.getter(proto, name, func(this vm.Value) (vm.Value, error) {
			s, ok := slotsOf[tPlainDateTime](this)
			if !ok {
				return vm.Undefined, r.typeErr("Method called on incompatible receiver: not a Temporal.PlainDateTime")
			}
			return f(s.dt), nil
		})
	}
	get("calendarId", func(temporal.DateTime) vm.Value { return vm.NewString("iso8601") })
	get("era", func(temporal.DateTime) vm.Value { return vm.Undefined })
	get("eraYear", func(temporal.DateTime) vm.Value { return vm.Undefined })
	get("year", func(dt temporal.DateTime) vm.Value { return num(dt.Date.Year) })
	get("month", func(dt temporal.DateTime) vm.Value { return num(dt.Date.Month) })
	get("monthCode", func(dt temporal.DateTime) vm.Value { return vm.NewString(fmt.Sprintf("M%02d", dt.Date.Month)) })
	get("day", func(dt temporal.DateTime) vm.Value { return num(dt.Date.Day) })
	get("hour", func(dt temporal.DateTime) vm.Value { return num(dt.Time.Hour) })
	get("minute", func(dt temporal.DateTime) vm.Value { return num(dt.Time.Minute) })
	get("second", func(dt temporal.DateTime) vm.Value { return num(dt.Time.Second) })
	get("millisecond", func(dt temporal.DateTime) vm.Value { return num(dt.Time.Millisecond) })
	get("microsecond", func(dt temporal.DateTime) vm.Value { return num(dt.Time.Microsecond) })
	get("nanosecond", func(dt temporal.DateTime) vm.Value { return num(dt.Time.Nanosecond) })
	get("dayOfWeek", func(dt temporal.DateTime) vm.Value { return num(dt.Date.DayOfWeek()) })
	get("dayOfYear", func(dt temporal.DateTime) vm.Value { return num(dt.Date.DayOfYear()) })
	get("weekOfYear", func(dt temporal.DateTime) vm.Value { w, _ := dt.Date.WeekOfYear(); return num(w) })
	get("yearOfWeek", func(dt temporal.DateTime) vm.Value { _, y := dt.Date.WeekOfYear(); return num(y) })
	get("daysInWeek", func(temporal.DateTime) vm.Value { return num(7) })
	get("daysInMonth", func(dt temporal.DateTime) vm.Value { return num(temporal.DaysInMonth(dt.Date.Year, dt.Date.Month)) })
	get("daysInYear", func(dt temporal.DateTime) vm.Value { return num(temporal.DaysInYear(dt.Date.Year)) })
	get("monthsInYear", func(temporal.DateTime) vm.Value { return num(12) })
	get("inLeapYear", func(dt temporal.DateTime) vm.Value { return vm.BooleanValue(temporal.IsLeapYear(dt.Date.Year)) })

	this := func() (*tPlainDateTime, error) { return thisSlots[tPlainDateTime](r, "PlainDateTime") }

	r.method(proto, "with", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		item := argAt(args, 0)
		if err := r.rejectObjectWithCalendarOrTimeZone(item); err != nil {
			return vm.Undefined, err
		}
		f, err := r.prepareFields(item, dateTimeFieldNames, nil, true)
		if err != nil {
			return vm.Undefined, err
		}
		opts, err := r.getOptionsObject(argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		overflow, err := r.overflowOption(opts)
		if err != nil {
			return vm.Undefined, err
		}
		cur := s.dt
		month, ok, err := r.monthFromFields(f)
		if err != nil {
			return vm.Undefined, err
		}
		if !ok {
			month = cur.Date.Month
		}
		pick := func(p *int, d int) *int {
			if p == nil {
				return &d
			}
			return p
		}
		merged := &tFields{
			Hour: pick(f.Hour, cur.Time.Hour), Minute: pick(f.Minute, cur.Time.Minute), Second: pick(f.Second, cur.Time.Second),
			Millisecond: pick(f.Millisecond, cur.Time.Millisecond), Microsecond: pick(f.Microsecond, cur.Time.Microsecond),
			Nanosecond: pick(f.Nanosecond, cur.Time.Nanosecond),
		}
		dt, err := r.regulateDateTime(*pick(f.Year, cur.Date.Year), month, *pick(f.Day, cur.Date.Day), merged, overflow)
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDateTime(dt)
	})

	r.method(proto, "withPlainTime", 0, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		var t temporal.Time
		if a := argAt(args, 0); !a.IsUndefined() {
			if t, err = r.toTemporalTime(a, vm.Undefined); err != nil {
				return vm.Undefined, err
			}
		}
		return r.createPlainDateTime(temporal.DateTime{Date: s.dt.Date, Time: t})
	})

	r.method(proto, "withCalendar", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		if err := r.calendarFromValue(argAt(args, 0)); err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDateTime(s.dt)
	})

	// AddDurationToDateTime
	addSub := func(name string, sign int) {
		r.method(proto, name, 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := this()
			if err != nil {
				return vm.Undefined, err
			}
			d, err := r.toTemporalDuration(argAt(args, 0))
			if err != nil {
				return vm.Undefined, err
			}
			if sign < 0 {
				d = d.Negated()
			}
			opts, err := r.getOptionsObject(argAt(args, 1))
			if err != nil {
				return vm.Undefined, err
			}
			overflow, err := r.overflowOption(opts)
			if err != nil {
				return vm.Undefined, err
			}
			in := temporal.ToInternalDuration(d)
			total := new(big.Int).Add(big.NewInt(s.dt.Time.Nanoseconds()), in.Time)
			carry, t := temporal.BalanceTime(total)
			date, err := temporal.AddISODate(s.dt.Date, in.Date.Years, in.Date.Months, in.Date.Weeks, in.Date.Days+carry, overflow)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			return r.createPlainDateTime(temporal.DateTime{Date: date, Time: t})
		})
	}
	addSub("add", 1)
	addSub("subtract", -1)

	// DifferenceTemporalPlainDateTime
	diff := func(name string, since bool) {
		r.method(proto, name, 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := this()
			if err != nil {
				return vm.Undefined, err
			}
			other, err := r.toTemporalDateTime(argAt(args, 0), vm.Undefined)
			if err != nil {
				return vm.Undefined, err
			}
			opts, err := r.getOptionsObject(argAt(args, 1))
			if err != nil {
				return vm.Undefined, err
			}
			set, err := r.differenceSettings(since, opts, unitsDateTime, nil, temporal.UnitNanosecond, temporal.UnitDay)
			if err != nil {
				return vm.Undefined, err
			}
			internal, err := temporal.DifferencePlainDateTimeWithRounding(s.dt, other, set.largest, set.increment, set.smallest, set.mode)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			d, err := internal.ToDuration(set.largest)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			if since {
				d = d.Negated()
			}
			return r.createDuration(d)
		})
	}
	diff("until", false)
	diff("since", true)

	r.method(proto, "round", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		roundTo := argAt(args, 0)
		if roundTo.IsUndefined() {
			return vm.Undefined, r.typeErr("round requires an options argument")
		}
		var smallestRaw string
		increment, mode := int64(1), temporal.RoundHalfExpand
		if roundTo.Type() == vm.TypeString {
			smallestRaw = roundTo.ToString()
		} else {
			opts, err := r.getOptionsObject(roundTo)
			if err != nil {
				return vm.Undefined, err
			}
			if increment, err = r.roundingIncrementOption(opts); err != nil {
				return vm.Undefined, err
			}
			if mode, err = r.roundingModeOption(opts, temporal.RoundHalfExpand); err != nil {
				return vm.Undefined, err
			}
			raw, present, err := r.rawUnitOption(opts, "smallestUnit")
			if err != nil {
				return vm.Undefined, err
			}
			if !present {
				return vm.Undefined, r.rangeErr("the smallestUnit option is required")
			}
			smallestRaw = raw
		}
		unit, _, err := r.validateUnit("smallestUnit", smallestRaw, true, unitsDateTime, false)
		if err != nil {
			return vm.Undefined, err
		}
		if unit < temporal.UnitDay {
			return vm.Undefined, r.rangeErr(smallestRaw + " is not a valid value for option smallestUnit")
		}
		max, inclusive := int64(1), true
		if unit != temporal.UnitDay {
			max, _ = temporal.MaximumTemporalDurationRoundingIncrement(unit)
			inclusive = false
		}
		if err := temporal.ValidateRoundingIncrement(increment, max, inclusive); err != nil {
			return vm.Undefined, r.err(err)
		}
		if unit == temporal.UnitNanosecond && increment == 1 {
			return r.createPlainDateTime(s.dt)
		}
		dt, err := temporal.RoundISODateTime(s.dt, increment, unit, mode)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return r.createPlainDateTime(dt)
	})

	r.method(proto, "equals", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		other, err := r.toTemporalDateTime(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(compareDateTime(s.dt, other) == 0), nil
	})

	r.method(proto, "toString", 0, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		opts, err := r.getOptionsObject(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		show, err := r.calendarNameOption(opts)
		if err != nil {
			return vm.Undefined, err
		}
		digits, err := r.fractionalSecondDigitsOption(opts)
		if err != nil {
			return vm.Undefined, err
		}
		mode, err := r.roundingModeOption(opts, temporal.RoundTrunc)
		if err != nil {
			return vm.Undefined, err
		}
		raw, present, err := r.rawUnitOption(opts, "smallestUnit")
		if err != nil {
			return vm.Undefined, err
		}
		smallest, ok, err := r.validateUnit("smallestUnit", raw, present, unitsTime, false)
		if err != nil {
			return vm.Undefined, err
		}
		if ok && smallest == temporal.UnitHour {
			return vm.Undefined, r.rangeErr("hour is not a valid value for option smallestUnit")
		}
		precision, unit, increment := secondsStringPrecision(smallest, ok, digits)
		dt, err := temporal.RoundISODateTime(s.dt, increment, unit, mode)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return vm.NewString(temporal.FormatDateTime(dt, precision) + calendarAnnotation(show)), nil
	})
	for _, name := range []string{"toLocaleString", "toJSON"} {
		r.method(proto, name, 0, func(_ vm.Value, _ []vm.Value) (vm.Value, error) {
			s, err := this()
			if err != nil {
				return vm.Undefined, err
			}
			return vm.NewString(temporal.FormatDateTime(s.dt, -1)), nil
		})
	}
	r.method(proto, "valueOf", 0, func(_ vm.Value, _ []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("Temporal.PlainDateTime cannot be converted to a primitive; use compare() or toString()")
	})

	r.method(proto, "toZonedDateTime", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		tz, err := r.toTemporalTimeZone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		opts, err := r.getOptionsObject(argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		dis, err := r.disambiguationOption(opts)
		if err != nil {
			return vm.Undefined, err
		}
		ns, err := tz.EpochNanosecondsFor(s.dt, dis)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return r.createZonedDateTime(ns, tz)
	})
	r.method(proto, "toPlainDate", 0, func(_ vm.Value, _ []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDate(s.dt.Date)
	})
	r.method(proto, "toPlainTime", 0, func(_ vm.Value, _ []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainTime(s.dt.Time)
	})
	return nil
}

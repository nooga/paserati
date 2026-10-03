package builtins

import (
	"fmt"
	"math/big"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.PlainYearMonth. The slot is a reference ISO date: the first of the
// month for everything but the constructor, which may be given another day.

func init() { registerTemporalInstaller(installPlainYearMonth) }

// ymWithinLimits is ISOYearMonthWithinLimits.
func ymWithinLimits(y, m int) bool {
	return (y > -271821 || (y == -271821 && m >= 4)) && (y < 275760 || (y == 275760 && m <= 9))
}

func (r *temporalRealm) createPlainYearMonth(ref temporal.Date) (vm.Value, error) {
	if !ymWithinLimits(ref.Year, ref.Month) {
		return vm.Undefined, r.rangeErr("year-month out of range")
	}
	return r.wrap("PlainYearMonth", &tPlainYearMonth{ref}), nil
}

// ---------------------------------------------------------------------------
// Helpers shared with temporal_plainmonthday.go
// ---------------------------------------------------------------------------

// ymdIsTemporalObject reports whether v has the internal slots of a type
// that carries a calendar.
func ymdIsTemporalObject(v vm.Value) bool {
	if _, ok := slotsOf[tPlainDate](v); ok {
		return true
	}
	if _, ok := slotsOf[tPlainDateTime](v); ok {
		return true
	}
	if _, ok := slotsOf[tPlainYearMonth](v); ok {
		return true
	}
	if _, ok := slotsOf[tPlainMonthDay](v); ok {
		return true
	}
	_, ok := slotsOf[tZoned](v)
	return ok
}

// ymdCheckCalendarString is ParseTemporalCalendarString: a calendar
// identifier or an ISO 8601 string with a calendar annotation.
func (r *temporalRealm) ymdCheckCalendarString(s string) error {
	if _, ok := temporal.CanonicalizeCalendarIdentifier(s); ok {
		return nil
	}
	for _, parse := range []func(string) (*temporal.Parsed, error){
		temporal.ParsePlainDateTimeString, temporal.ParseInstantString,
		temporal.ParsePlainYearMonthString, temporal.ParsePlainMonthDayString,
	} {
		if p, err := parse(s); err == nil {
			return r.ymdCheckParsedCalendar(p)
		}
	}
	return r.rangeErr("invalid calendar " + s)
}

func (r *temporalRealm) ymdCheckParsedCalendar(p *temporal.Parsed) error {
	if p.Calendar == "" {
		return nil
	}
	_, err := r.canonicalCalendar(p.Calendar)
	return err
}

// ymdReadCalendar is GetTemporalCalendarIdentifierWithISODefault: Temporal
// objects are iso8601, anything else has its "calendar" property converted
// by ToTemporalCalendarIdentifier.
func (r *temporalRealm) ymdReadCalendar(item vm.Value) error {
	if ymdIsTemporalObject(item) {
		return nil
	}
	c, err := r.vm.GetProperty(item, "calendar")
	if err != nil || c.IsUndefined() {
		return err
	}
	if ymdIsTemporalObject(c) {
		return nil
	}
	if c.Type() != vm.TypeString {
		return r.typeErr("calendar must be a string")
	}
	return r.ymdCheckCalendarString(c.ToString())
}

// ymdRejectCalendarOrTimeZone is RejectObjectWithCalendarOrTimeZone.
func (r *temporalRealm) ymdRejectCalendarOrTimeZone(item vm.Value) error {
	if !isObjectValue(item) || ymdIsTemporalObject(item) {
		return r.typeErr("argument must be a plain object")
	}
	for _, name := range []string{"calendar", "timeZone"} {
		v, err := r.vm.GetProperty(item, name)
		if err != nil {
			return err
		}
		if !v.IsUndefined() {
			return r.typeErr("the " + name + " property is not allowed here")
		}
	}
	return nil
}

// ymdConstructorCalendar converts the constructors' calendar argument.
func (r *temporalRealm) ymdConstructorCalendar(v vm.Value) error {
	if v.IsUndefined() {
		return nil
	}
	if v.Type() != vm.TypeString {
		return r.typeErr("calendar must be a string")
	}
	_, err := r.canonicalCalendar(v.ToString())
	return err
}

// ymdMergeMonth is CalendarMergeFields' month handling: a partial month or
// monthCode replaces both of the base's.
func ymdMergeMonth(partial *tFields, baseMonth int) *tFields {
	m := &tFields{Year: partial.Year, Day: partial.Day, Month: partial.Month, MonthCode: partial.MonthCode}
	if m.Month == nil && m.MonthCode == nil {
		code := fmt.Sprintf("M%02d", baseMonth)
		m.MonthCode = &code
	}
	return m
}

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

// yearMonthFromFields is CalendarYearMonthFromFields for ISO: the reference
// day is always 1.
func (r *temporalRealm) yearMonthFromFields(f *tFields, overflow temporal.Overflow) (temporal.Date, error) {
	if f.Year == nil {
		return temporal.Date{}, r.typeErr("missing required property year")
	}
	m, ok, err := r.monthFromFields(f)
	if err != nil {
		return temporal.Date{}, err
	}
	if !ok {
		return temporal.Date{}, r.typeErr("missing required property month or monthCode")
	}
	d, err := temporal.RegulateISODate(*f.Year, m, 1, overflow)
	if err != nil {
		return temporal.Date{}, r.err(err)
	}
	if !ymWithinLimits(d.Year, d.Month) {
		return temporal.Date{}, r.rangeErr("year-month out of range")
	}
	return d, nil
}

// toTemporalYearMonth is ToTemporalYearMonth.
func (r *temporalRealm) toTemporalYearMonth(item, options vm.Value) (temporal.Date, error) {
	if isObjectValue(item) {
		if s, ok := slotsOf[tPlainYearMonth](item); ok {
			opts, err := r.getOptionsObject(options)
			if err != nil {
				return temporal.Date{}, err
			}
			if _, err := r.overflowOption(opts); err != nil {
				return temporal.Date{}, err
			}
			return s.date, nil
		}
		if err := r.ymdReadCalendar(item); err != nil {
			return temporal.Date{}, err
		}
		f, err := r.prepareFields(item, []string{fMonth, fMonthCode, fYear}, nil, false)
		if err != nil {
			return temporal.Date{}, err
		}
		opts, err := r.getOptionsObject(options)
		if err != nil {
			return temporal.Date{}, err
		}
		overflow, err := r.overflowOption(opts)
		if err != nil {
			return temporal.Date{}, err
		}
		return r.yearMonthFromFields(f, overflow)
	}
	if item.Type() != vm.TypeString {
		return temporal.Date{}, r.typeErr("a year-month must be an object or a string")
	}
	p, err := temporal.ParsePlainYearMonthString(item.ToString())
	if err != nil {
		return temporal.Date{}, r.err(err)
	}
	if err := r.ymdCheckParsedCalendar(p); err != nil {
		return temporal.Date{}, err
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return temporal.Date{}, err
	}
	if _, err := r.overflowOption(opts); err != nil {
		return temporal.Date{}, err
	}
	if !ymWithinLimits(p.Date.Year, p.Date.Month) {
		return temporal.Date{}, r.rangeErr("year-month out of range")
	}
	return temporal.Date{Year: p.Date.Year, Month: p.Date.Month, Day: 1}, nil
}

// ---------------------------------------------------------------------------
// Installation
// ---------------------------------------------------------------------------

func installPlainYearMonth(r *temporalRealm) error {
	ctor, proto := r.newClass("PlainYearMonth", 2, func(args []vm.Value, p vm.Value) (vm.Value, error) {
		y, err := r.toIntegerWithTruncation(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		m, err := r.toIntegerWithTruncation(argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		if err := r.ymdConstructorCalendar(argAt(args, 2)); err != nil {
			return vm.Undefined, err
		}
		day := 1.0
		if v := argAt(args, 3); !v.IsUndefined() {
			if day, err = r.toIntegerWithTruncation(v); err != nil {
				return vm.Undefined, err
			}
		}
		ref := temporal.Date{Year: clampToInt(y), Month: clampToInt(m), Day: clampToInt(day)}
		if !temporal.IsValidISODate(ref.Year, ref.Month, ref.Day) || !ymWithinLimits(ref.Year, ref.Month) {
			return vm.Undefined, r.rangeErr("invalid year-month")
		}
		return r.wrapWithProto(p, &tPlainYearMonth{ref}), nil
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		d, err := r.toTemporalYearMonth(argAt(args, 0), argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainYearMonth(d)
	})
	r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
		a, err := r.toTemporalYearMonth(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		b, err := r.toTemporalYearMonth(argAt(args, 1), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(temporal.CompareISODate(a, b))), nil
	})

	this := func() (*tPlainYearMonth, error) { return thisSlots[tPlainYearMonth](r, "PlainYearMonth") }
	get := func(name string, f func(d temporal.Date) vm.Value) {
		r.getter(proto, name, func(vm.Value) (vm.Value, error) {
			s, err := this()
			if err != nil {
				return vm.Undefined, err
			}
			return f(s.date), nil
		})
	}
	get("calendarId", func(temporal.Date) vm.Value { return vm.NewString("iso8601") })
	get("era", func(temporal.Date) vm.Value { return vm.Undefined })
	get("eraYear", func(temporal.Date) vm.Value { return vm.Undefined })
	get("year", func(d temporal.Date) vm.Value { return vm.NumberValue(float64(d.Year)) })
	get("month", func(d temporal.Date) vm.Value { return vm.NumberValue(float64(d.Month)) })
	get("monthCode", func(d temporal.Date) vm.Value { return vm.NewString(fmt.Sprintf("M%02d", d.Month)) })
	get("daysInMonth", func(d temporal.Date) vm.Value { return vm.NumberValue(float64(temporal.DaysInMonth(d.Year, d.Month))) })
	get("daysInYear", func(d temporal.Date) vm.Value { return vm.NumberValue(float64(temporal.DaysInYear(d.Year))) })
	get("monthsInYear", func(temporal.Date) vm.Value { return vm.NumberValue(12) })
	get("inLeapYear", func(d temporal.Date) vm.Value { return vm.BooleanValue(temporal.IsLeapYear(d.Year)) })

	r.method(proto, "with", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		item := argAt(args, 0)
		if err := r.ymdRejectCalendarOrTimeZone(item); err != nil {
			return vm.Undefined, err
		}
		partial, err := r.prepareFields(item, []string{fMonth, fMonthCode, fYear}, nil, true)
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
		merged := ymdMergeMonth(partial, s.date.Month)
		if merged.Year == nil {
			merged.Year = &s.date.Year
		}
		d, err := r.yearMonthFromFields(merged, overflow)
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainYearMonth(d)
	})

	// AddDurationToYearMonth: a negative duration starts from the last day of
	// the month so that month-end clamping behaves as it does for dates.
	addSub := func(sign int) func(vm.Value, []vm.Value) (vm.Value, error) {
		return func(_ vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := this()
			if err != nil {
				return vm.Undefined, err
			}
			dur, err := r.toTemporalDuration(argAt(args, 0))
			if err != nil {
				return vm.Undefined, err
			}
			if sign < 0 {
				dur = dur.Negated()
			}
			opts, err := r.getOptionsObject(argAt(args, 1))
			if err != nil {
				return vm.Undefined, err
			}
			overflow, err := r.overflowOption(opts)
			if err != nil {
				return vm.Undefined, err
			}
			date := temporal.Date{Year: s.date.Year, Month: s.date.Month, Day: 1}
			if !temporal.ISODateWithinLimits(date) {
				return vm.Undefined, r.rangeErr("date out of range")
			}
			if temporal.DurationSign(dur) < 0 {
				next, err := temporal.AddISODate(date, 0, 1, 0, 0, temporal.OverflowConstrain)
				if err != nil {
					return vm.Undefined, r.err(err)
				}
				if date = temporal.BalanceISODate(next.Year, next.Month, int64(next.Day)-1); !temporal.ISODateWithinLimits(date) {
					return vm.Undefined, r.rangeErr("date out of range")
				}
			}
			// ToDateDurationRecordWithoutTime: time units fold into whole days.
			id := temporal.ToInternalDuration(dur)
			days := id.Date.Days + new(big.Int).Quo(id.Time, big.NewInt(temporal.UnitDay.NanosecondsPerUnit())).Int64()
			added, err := temporal.AddISODate(date, id.Date.Years, id.Date.Months, id.Date.Weeks, days, overflow)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			return r.createPlainYearMonth(temporal.Date{Year: added.Year, Month: added.Month, Day: 1})
		}
	}
	r.method(proto, "add", 1, addSub(1))
	r.method(proto, "subtract", 1, addSub(-1))

	// DifferenceTemporalPlainYearMonth.
	diff := func(since bool) func(vm.Value, []vm.Value) (vm.Value, error) {
		return func(_ vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := this()
			if err != nil {
				return vm.Undefined, err
			}
			other, err := r.toTemporalYearMonth(argAt(args, 0), vm.Undefined)
			if err != nil {
				return vm.Undefined, err
			}
			opts, err := r.getOptionsObject(argAt(args, 1))
			if err != nil {
				return vm.Undefined, err
			}
			set, err := r.differenceSettings(since, opts, unitsDate, []temporal.Unit{temporal.UnitWeek, temporal.UnitDay}, temporal.UnitMonth, temporal.UnitYear)
			if err != nil {
				return vm.Undefined, err
			}
			if temporal.CompareISODate(s.date, other) == 0 {
				return r.createDuration(temporal.Duration{})
			}
			thisDate := temporal.Date{Year: s.date.Year, Month: s.date.Month, Day: 1}
			otherDate := temporal.Date{Year: other.Year, Month: other.Month, Day: 1}
			if !temporal.ISODateWithinLimits(thisDate) || !temporal.ISODateWithinLimits(otherDate) {
				return vm.Undefined, r.rangeErr("date out of range")
			}
			dd := temporal.DifferenceISODate(thisDate, otherDate, set.largest)
			id := temporal.InternalDuration{Date: dd, Time: new(big.Int)}
			if set.smallest != temporal.UnitMonth || set.increment != 1 {
				dest := temporal.GetUTCEpochNanoseconds(temporal.DateTime{Date: otherDate})
				id, err = temporal.RoundRelativeDuration(id, dest, temporal.DateTime{Date: thisDate}, nil, set.largest, set.increment, set.smallest, set.mode)
				if err != nil {
					return vm.Undefined, r.err(err)
				}
			}
			res, err := id.ToDuration(temporal.UnitDay)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			if since {
				res = res.Negated()
			}
			return r.createDuration(res)
		}
	}
	r.method(proto, "until", 1, diff(false))
	r.method(proto, "since", 1, diff(true))

	r.method(proto, "equals", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		other, err := r.toTemporalYearMonth(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(temporal.CompareISODate(s.date, other) == 0), nil
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
		return vm.NewString(formatYearMonth(s.date, show)), nil
	})
	plain := func(_ vm.Value, _ []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(formatYearMonth(s.date, "auto")), nil
	}
	r.method(proto, "toJSON", 0, plain)
	r.method(proto, "toLocaleString", 0, plain)
	r.method(proto, "valueOf", 0, func(vm.Value, []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("Temporal.PlainYearMonth cannot be converted to a primitive; use compare() or equals()")
	})

	r.method(proto, "toPlainDate", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		item := argAt(args, 0)
		if !isObjectValue(item) {
			return vm.Undefined, r.typeErr("toPlainDate requires an object")
		}
		f, err := r.prepareFields(item, []string{fDay}, []string{fDay}, false)
		if err != nil {
			return vm.Undefined, err
		}
		d, err := temporal.RegulateISODate(s.date.Year, s.date.Month, *f.Day, temporal.OverflowConstrain)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return r.createPlainDate(d)
	})
	return nil
}

// formatYearMonth is TemporalYearMonthToString. The reference day only
// shows when the calendar annotation does.
func formatYearMonth(ref temporal.Date, show string) string {
	if show == "always" || show == "critical" {
		return temporal.FormatDate(ref) + ymdCalendarAnnotation(show)
	}
	return fmt.Sprintf("%s-%02d", temporal.FormatYear(ref.Year), ref.Month)
}

// ymdCalendarAnnotation is FormatCalendarAnnotation for the ISO calendar.
func ymdCalendarAnnotation(show string) string {
	switch show {
	case "always":
		return "[u-ca=iso8601]"
	case "critical":
		return "[!u-ca=iso8601]"
	}
	return ""
}

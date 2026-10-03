package builtins

import (
	"math/big"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.PlainDate (ISO 8601 calendar only).

func init() { registerTemporalInstaller(installPlainDate) }

var plainDateFieldNames = []string{fYear, fMonth, fMonthCode, fDay}

func monthCode(m int) string { return "M" + string(rune('0'+m/10)) + string(rune('0'+m%10)) }

// pdStartOfDay is GetStartOfDay: the first instant of a date in a zone,
// which is the transition itself when midnight is skipped.
func pdStartOfDay(tz temporal.TimeZone, d temporal.Date) (*big.Int, error) {
	midnight := temporal.DateTime{Date: d}
	if p := tz.PossibleEpochNanoseconds(midnight); len(p) > 0 {
		if !temporal.InstantWithinLimits(p[0]) {
			return nil, &temporal.RangeError{Msg: "date-time is outside the representable range"}
		}
		return p[0], nil
	}
	e, err := tz.EpochNanosecondsFor(midnight, temporal.DisambiguateEarlier)
	if err != nil {
		return nil, err
	}
	t, ok := tz.NextTransition(e)
	if !ok {
		return nil, &temporal.RangeError{Msg: "no start of day"}
	}
	return t, nil
}

func (r *temporalRealm) createPlainDateWithProto(d temporal.Date, proto protoRef) (vm.Value, error) {
	if !temporal.IsValidISODate(d.Year, d.Month, d.Day) || !temporal.ISODateWithinLimits(d) {
		return vm.Undefined, r.rangeErr("date is outside the supported range")
	}
	return r.wrapNew(proto, &tPlainDate{d})
}

func (r *temporalRealm) createPlainDate(d temporal.Date) (vm.Value, error) {
	return r.createPlainDateWithProto(d, staticProto(r.protos["PlainDate"]))
}

// dateFromFields is CalendarDateFromFields once the fields have been read.
func (r *temporalRealm) dateFromFields(year, month, day int, overflow temporal.Overflow) (temporal.Date, error) {
	d, err := temporal.RegulateISODate(year, month, day, overflow)
	if err != nil {
		return d, r.err(err)
	}
	if !temporal.ISODateWithinLimits(d) {
		return d, r.rangeErr("date is outside the supported range")
	}
	return d, nil
}

// toTemporalDate is ToTemporalDate.
func (r *temporalRealm) toTemporalDate(item, options vm.Value) (temporal.Date, error) {
	if isObjectValue(item) {
		var d temporal.Date
		known := true
		if s, ok := slotsOf[tPlainDate](item); ok {
			d = s.date
		} else if s, ok := slotsOf[tPlainDateTime](item); ok {
			d = s.dt.Date
		} else if s, ok := slotsOf[tZoned](item); ok {
			d = s.tz.LocalDateTime(s.ns).Date
		} else {
			known = false
		}
		if known {
			opts, err := r.getOptionsObject(options)
			if err != nil {
				return d, err
			}
			_, err = r.overflowOption(opts)
			return d, err
		}
		cal, err := r.vm.GetProperty(item, "calendar")
		if err != nil {
			return d, err
		}
		if !cal.IsUndefined() {
			if err := r.checkCalendar(cal); err != nil {
				return d, err
			}
		}
		f, err := r.prepareFields(item, plainDateFieldNames, []string{fYear, fDay}, false)
		if err != nil {
			return d, err
		}
		opts, err := r.getOptionsObject(options)
		if err != nil {
			return d, err
		}
		overflow, err := r.overflowOption(opts)
		if err != nil {
			return d, err
		}
		month, ok, err := r.monthFromFields(f)
		if err != nil {
			return d, err
		}
		if !ok {
			return d, r.typeErr("missing required property month or monthCode")
		}
		return r.dateFromFields(*f.Year, month, *f.Day, overflow)
	}
	if item.Type() != vm.TypeString {
		return temporal.Date{}, r.typeErr("cannot convert value to a PlainDate")
	}
	p, err := temporal.ParsePlainDateString(item.ToString())
	if err != nil {
		return temporal.Date{}, r.err(err)
	}
	if p.Calendar != "" {
		if _, err := r.canonicalCalendar(p.Calendar); err != nil {
			return p.Date, err
		}
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return p.Date, err
	}
	if _, err = r.overflowOption(opts); err != nil {
		return p.Date, err
	}
	if !temporal.ISODateWithinLimits(p.Date) {
		return p.Date, r.rangeErr("date is outside the supported range")
	}
	return p.Date, nil
}

// toDateDuration is ToDateDurationRecordWithoutTime: time units fold into
// days as 24-hour days.
func (r *temporalRealm) pdDateDuration(d temporal.Duration) (years, months, weeks, days int64, err error) {
	id := temporal.ToInternalDuration(d)
	extra := new(big.Int).Quo(id.Time, big.NewInt(86400*1e9))
	if !extra.IsInt64() {
		return 0, 0, 0, 0, r.rangeErr("duration out of range")
	}
	return id.Date.Years, id.Date.Months, id.Date.Weeks, id.Date.Days + extra.Int64(), nil
}

func installPlainDate(r *temporalRealm) error {
	ctor, proto := r.newClass("PlainDate", 3, func(args []vm.Value, p protoRef) (vm.Value, error) {
		var n [3]float64
		for i := range n {
			v, err := r.toIntegerWithTruncation(argAt(args, i))
			if err != nil {
				return vm.Undefined, err
			}
			n[i] = v
		}
		if cal := argAt(args, 3); !cal.IsUndefined() {
			if cal.Type() != vm.TypeString {
				return vm.Undefined, r.typeErr("calendar must be a string")
			}
			if _, err := r.canonicalCalendar(cal.ToString()); err != nil {
				return vm.Undefined, err
			}
		}
		return r.createPlainDateWithProto(temporal.Date{Year: clampToInt(n[0]), Month: clampToInt(n[1]), Day: clampToInt(n[2])}, p)
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		d, err := r.toTemporalDate(argAt(args, 0), argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDate(d)
	})
	r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
		a, err := r.toTemporalDate(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		b, err := r.toTemporalDate(argAt(args, 1), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(temporal.CompareISODate(a, b))), nil
	})

	// Getters.
	num := func(f func(d temporal.Date) int) func(d temporal.Date) vm.Value {
		return func(d temporal.Date) vm.Value { return vm.NumberValue(float64(f(d))) }
	}
	getters := []struct {
		name string
		f    func(d temporal.Date) vm.Value
	}{
		{"calendarId", func(temporal.Date) vm.Value { return vm.NewString("iso8601") }},
		{"era", func(temporal.Date) vm.Value { return vm.Undefined }},
		{"eraYear", func(temporal.Date) vm.Value { return vm.Undefined }},
		{"year", num(func(d temporal.Date) int { return d.Year })},
		{"month", num(func(d temporal.Date) int { return d.Month })},
		{"monthCode", func(d temporal.Date) vm.Value { return vm.NewString(monthCode(d.Month)) }},
		{"day", num(func(d temporal.Date) int { return d.Day })},
		{"dayOfWeek", num(temporal.Date.DayOfWeek)},
		{"dayOfYear", num(temporal.Date.DayOfYear)},
		{"weekOfYear", num(func(d temporal.Date) int { w, _ := d.WeekOfYear(); return w })},
		{"yearOfWeek", num(func(d temporal.Date) int { _, y := d.WeekOfYear(); return y })},
		{"daysInWeek", num(func(temporal.Date) int { return 7 })},
		{"daysInMonth", num(func(d temporal.Date) int { return temporal.DaysInMonth(d.Year, d.Month) })},
		{"daysInYear", num(func(d temporal.Date) int { return temporal.DaysInYear(d.Year) })},
		{"monthsInYear", num(func(temporal.Date) int { return 12 })},
		{"inLeapYear", func(d temporal.Date) vm.Value { return vm.BooleanValue(temporal.IsLeapYear(d.Year)) }},
	}
	for _, g := range getters {
		g := g
		r.getter(proto, g.name, func(this vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainDate](r, "PlainDate")
			if err != nil {
				return vm.Undefined, err
			}
			return g.f(s.date), nil
		})
	}

	r.method(proto, "with", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		like := argAt(args, 0)
		if !isObjectValue(like) {
			return vm.Undefined, r.typeErr("with() argument must be an object")
		}
		// RejectObjectWithCalendarOrTimeZone
		_, isDate := slotsOf[tPlainDate](like)
		_, isDT := slotsOf[tPlainDateTime](like)
		_, isYM := slotsOf[tPlainYearMonth](like)
		_, isMD := slotsOf[tPlainMonthDay](like)
		_, isZ := slotsOf[tZoned](like)
		if isDate || isDT || isYM || isMD || isZ {
			return vm.Undefined, r.typeErr("with() argument must not be a Temporal object")
		}
		for _, name := range []string{"calendar", "timeZone"} {
			v, err := r.vm.GetProperty(like, name)
			if err != nil {
				return vm.Undefined, err
			}
			if !v.IsUndefined() {
				return vm.Undefined, r.typeErr("with() argument must not have a " + name + " property")
			}
		}
		f, err := r.prepareFields(like, plainDateFieldNames, nil, true)
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
		year, month, day := s.date.Year, s.date.Month, s.date.Day
		if f.Year != nil {
			year = *f.Year
		}
		if f.Day != nil {
			day = *f.Day
		}
		if m, ok, err := r.monthFromFields(f); err != nil {
			return vm.Undefined, err
		} else if ok {
			month = m
		}
		d, err := r.dateFromFields(year, month, day, overflow)
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDate(d)
	})

	r.method(proto, "withCalendar", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		if err := r.checkCalendar(argAt(args, 0)); err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDate(s.date)
	})

	addSub := func(name string, sign int64) {
		r.method(proto, name, 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainDate](r, "PlainDate")
			if err != nil {
				return vm.Undefined, err
			}
			dur, err := r.toTemporalDuration(argAt(args, 0))
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
			y, m, w, d, err := r.pdDateDuration(dur)
			if err != nil {
				return vm.Undefined, err
			}
			res, err := temporal.AddISODate(s.date, sign*y, sign*m, sign*w, sign*d, overflow)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			return r.createPlainDate(res)
		})
	}
	addSub("add", 1)
	addSub("subtract", -1)

	diff := func(name string, since bool) {
		r.method(proto, name, 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainDate](r, "PlainDate")
			if err != nil {
				return vm.Undefined, err
			}
			other, err := r.toTemporalDate(argAt(args, 0), vm.Undefined)
			if err != nil {
				return vm.Undefined, err
			}
			opts, err := r.getOptionsObject(argAt(args, 1))
			if err != nil {
				return vm.Undefined, err
			}
			set, err := r.differenceSettings(since, opts, unitsDate, nil, temporal.UnitDay, temporal.UnitDay)
			if err != nil {
				return vm.Undefined, err
			}
			if temporal.CompareISODate(s.date, other) == 0 {
				return r.createDuration(temporal.Duration{})
			}
			var id temporal.InternalDuration
			if set.smallest == temporal.UnitDay && set.increment == 1 {
				id = temporal.InternalDuration{Date: temporal.DifferenceISODate(s.date, other, set.largest), Time: new(big.Int)}
			} else {
				id, err = temporal.DifferencePlainDateTimeWithRounding(temporal.DateTime{Date: s.date}, temporal.DateTime{Date: other},
					set.largest, set.increment, set.smallest, set.mode)
				if err != nil {
					return vm.Undefined, r.err(err)
				}
			}
			d, err := id.ToDuration(temporal.UnitDay)
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

	r.method(proto, "equals", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		other, err := r.toTemporalDate(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(s.date == other), nil
	})

	r.method(proto, "toPlainDateTime", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		var t temporal.Time
		if item := argAt(args, 0); !item.IsUndefined() {
			if t, err = r.toTemporalTime(item, vm.Undefined); err != nil {
				return vm.Undefined, err
			}
		}
		return r.createPlainDateTime(temporal.DateTime{Date: s.date, Time: t})
	})

	r.method(proto, "toZonedDateTime", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		item, plainTime := argAt(args, 0), vm.Undefined
		tzItem := item
		if isObjectValue(item) {
			tzLike, err := r.vm.GetProperty(item, "timeZone")
			if err != nil {
				return vm.Undefined, err
			}
			if !tzLike.IsUndefined() {
				tzItem = tzLike
				if plainTime, err = r.vm.GetProperty(item, "plainTime"); err != nil {
					return vm.Undefined, err
				}
			}
		}
		tz, err := r.toTemporalTimeZone(tzItem)
		if err != nil {
			return vm.Undefined, err
		}
		var ns *big.Int
		if plainTime.IsUndefined() {
			ns, err = pdStartOfDay(tz, s.date)
		} else {
			var t temporal.Time
			if t, err = r.toTemporalTime(plainTime, vm.Undefined); err != nil {
				return vm.Undefined, err
			}
			ns, err = tz.EpochNanosecondsFor(temporal.DateTime{Date: s.date, Time: t}, temporal.DisambiguateCompatible)
		}
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return r.createZonedDateTime(ns, tz)
	})

	r.method(proto, "toPlainYearMonth", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainYearMonth(temporal.Date{Year: s.date.Year, Month: s.date.Month, Day: 1})
	})
	r.method(proto, "toPlainMonthDay", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainMonthDay(temporal.Date{Year: 1972, Month: s.date.Month, Day: s.date.Day})
	})

	r.method(proto, "toString", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tPlainDate](r, "PlainDate")
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
		out := temporal.FormatDate(s.date)
		switch show {
		case "always":
			out += "[u-ca=iso8601]"
		case "critical":
			out += "[!u-ca=iso8601]"
		}
		return vm.NewString(out), nil
	})
	plain := func(name string) {
		r.method(proto, name, 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainDate](r, "PlainDate")
			if err != nil {
				return vm.Undefined, err
			}
			return vm.NewString(temporal.FormatDate(s.date)), nil
		})
	}
	plain("toLocaleString")
	plain("toJSON")
	r.method(proto, "valueOf", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("Temporal.PlainDate cannot be converted to a primitive; use compare() or equals()")
	})
	return nil
}

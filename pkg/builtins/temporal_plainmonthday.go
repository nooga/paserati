package builtins

import (
	"fmt"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.PlainMonthDay. The slot is a reference ISO date whose year is 1972
// (a leap year, so February 29 exists) unless the constructor is given one.

func init() { registerTemporalInstaller(installPlainMonthDay) }

const mdReferenceYear = 1972

func (r *temporalRealm) createPlainMonthDay(ref temporal.Date) (vm.Value, error) {
	if !temporal.ISODateWithinLimits(ref) {
		return vm.Undefined, r.rangeErr("month-day out of range")
	}
	return r.wrap("PlainMonthDay", &tPlainMonthDay{ref}), nil
}

// monthDayFromFields is CalendarMonthDayFromFields for ISO. A year, when
// given, only decides how the day is constrained or rejected.
func (r *temporalRealm) monthDayFromFields(f *tFields, overflow temporal.Overflow) (temporal.Date, error) {
	if f.Day == nil {
		return temporal.Date{}, r.typeErr("missing required property day")
	}
	m, ok, err := r.monthFromFields(f)
	if err != nil {
		return temporal.Date{}, err
	}
	if !ok {
		return temporal.Date{}, r.typeErr("missing required property month or monthCode")
	}
	year := mdReferenceYear
	if f.Year != nil {
		year = *f.Year
	}
	d, err := temporal.RegulateISODate(year, m, *f.Day, overflow)
	if err != nil {
		return temporal.Date{}, r.err(err)
	}
	return temporal.Date{Year: mdReferenceYear, Month: d.Month, Day: d.Day}, nil
}

// toTemporalMonthDay is ToTemporalMonthDay.
func (r *temporalRealm) toTemporalMonthDay(item, options vm.Value) (temporal.Date, error) {
	if isObjectValue(item) {
		if s, ok := slotsOf[tPlainMonthDay](item); ok {
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
		f, err := r.prepareFields(item, []string{fDay, fMonth, fMonthCode, fYear}, nil, false)
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
		return r.monthDayFromFields(f, overflow)
	}
	if item.Type() != vm.TypeString {
		return temporal.Date{}, r.typeErr("a month-day must be an object or a string")
	}
	p, err := temporal.ParsePlainMonthDayString(item.ToString())
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
	return temporal.Date{Year: mdReferenceYear, Month: p.Date.Month, Day: p.Date.Day}, nil
}

func installPlainMonthDay(r *temporalRealm) error {
	ctor, proto := r.newClass("PlainMonthDay", 2, func(args []vm.Value, p vm.Value) (vm.Value, error) {
		m, err := r.toIntegerWithTruncation(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		d, err := r.toIntegerWithTruncation(argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		if err := r.ymdConstructorCalendar(argAt(args, 2)); err != nil {
			return vm.Undefined, err
		}
		year := float64(mdReferenceYear)
		if v := argAt(args, 3); !v.IsUndefined() {
			if year, err = r.toIntegerWithTruncation(v); err != nil {
				return vm.Undefined, err
			}
		}
		ref := temporal.Date{Year: clampToInt(year), Month: clampToInt(m), Day: clampToInt(d)}
		if !temporal.IsValidISODate(ref.Year, ref.Month, ref.Day) || !temporal.ISODateWithinLimits(ref) {
			return vm.Undefined, r.rangeErr("invalid month-day")
		}
		return r.wrapWithProto(p, &tPlainMonthDay{ref}), nil
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		d, err := r.toTemporalMonthDay(argAt(args, 0), argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainMonthDay(d)
	})

	this := func() (*tPlainMonthDay, error) { return thisSlots[tPlainMonthDay](r, "PlainMonthDay") }
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
	get("monthCode", func(d temporal.Date) vm.Value { return vm.NewString(fmt.Sprintf("M%02d", d.Month)) })
	get("day", func(d temporal.Date) vm.Value { return vm.NumberValue(float64(d.Day)) })

	r.method(proto, "with", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		item := argAt(args, 0)
		if err := r.ymdRejectCalendarOrTimeZone(item); err != nil {
			return vm.Undefined, err
		}
		partial, err := r.prepareFields(item, []string{fDay, fMonth, fMonthCode, fYear}, nil, true)
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
		if merged.Day == nil {
			merged.Day = &s.date.Day
		}
		d, err := r.monthDayFromFields(merged, overflow)
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainMonthDay(d)
	})

	r.method(proto, "equals", 1, func(_ vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		other, err := r.toTemporalMonthDay(argAt(args, 0), vm.Undefined)
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
		return vm.NewString(formatMonthDay(s.date, show)), nil
	})
	plain := func(_ vm.Value, _ []vm.Value) (vm.Value, error) {
		s, err := this()
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewString(formatMonthDay(s.date, "auto")), nil
	}
	r.method(proto, "toJSON", 0, plain)
	r.method(proto, "toLocaleString", 0, plain)
	r.method(proto, "valueOf", 0, func(vm.Value, []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("Temporal.PlainMonthDay cannot be converted to a primitive; use equals()")
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
		f, err := r.prepareFields(item, []string{fYear}, []string{fYear}, false)
		if err != nil {
			return vm.Undefined, err
		}
		d, err := temporal.RegulateISODate(*f.Year, s.date.Month, s.date.Day, temporal.OverflowConstrain)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return r.createPlainDate(d)
	})
	return nil
}

// formatMonthDay is TemporalMonthDayToString: the reference year only shows
// when the calendar annotation does.
func formatMonthDay(ref temporal.Date, show string) string {
	if show == "always" || show == "critical" {
		return temporal.FormatDate(ref) + ymdCalendarAnnotation(show)
	}
	return fmt.Sprintf("%02d-%02d", ref.Month, ref.Day)
}

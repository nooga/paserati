package builtins

import (
	"fmt"
	"math/big"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.ZonedDateTime: an exact instant plus a time zone (the ISO 8601
// calendar only). The slots are tZoned in temporal_common.go.

func init() {
	registerTemporalInstaller(installZonedDateTime)
}

var zonedFieldNames = []string{fYear, fMonth, fMonthCode, fDay, fHour, fMinute, fSecond, fMillisecond, fMicrosecond, fNanosecond, fOffset}

// ---------------------------------------------------------------------------
// Cross-type operations (temporal_contract.go)
// ---------------------------------------------------------------------------

func (r *temporalRealm) createZonedDateTime(ns *big.Int, tz temporal.TimeZone) (vm.Value, error) {
	if !temporal.InstantWithinLimits(ns) {
		return vm.Undefined, r.rangeErr("epoch nanoseconds out of range")
	}
	return r.wrap("ZonedDateTime", &tZoned{ns: ns, tz: tz}), nil
}

// toTemporalTimeZone is ToTemporalTimeZoneIdentifier.
func (r *temporalRealm) toTemporalTimeZone(item vm.Value) (temporal.TimeZone, error) {
	if z, ok := slotsOf[tZoned](item); ok {
		return z.tz, nil
	}
	if item.Type() != vm.TypeString {
		return temporal.TimeZone{}, r.typeErr("time zone must be a string")
	}
	s := item.ToString()
	if _, _, _, err := temporal.ParseTimeZoneIdentifier(s); err == nil {
		tz, err := temporal.ParseTimeZone(s)
		return tz, r.err(err)
	}
	// ParseTemporalTimeZoneString: an ISO string with an annotation, "Z" or an offset.
	p, err := temporal.ParseZonedDateTimeString(s)
	if err != nil {
		var err2 error
		if p, err2 = temporal.ParseInstantString(s); err2 != nil {
			return temporal.TimeZone{}, r.err(err)
		}
	}
	var id string
	switch {
	case p.TimeZone != "":
		id = p.TimeZone
	case p.UTC:
		id = "UTC"
	case p.HasOffset && !p.OffsetHasSubMinute:
		id = temporal.FormatOffset(p.OffsetNs, true)
	default:
		return temporal.TimeZone{}, r.rangeErr("invalid time zone string " + s)
	}
	tz, err := temporal.ParseTimeZone(id)
	return tz, r.err(err)
}

// toTemporalZonedDateTime is ToTemporalZonedDateTime.
func (r *temporalRealm) toTemporalZonedDateTime(item, options vm.Value) (*tZoned, error) {
	if isObjectValue(item) {
		if z, ok := slotsOf[tZoned](item); ok {
			opts, err := r.getOptionsObject(options)
			if err != nil {
				return nil, err
			}
			if _, err = r.disambiguationOption(opts); err != nil {
				return nil, err
			}
			if _, err = r.offsetOption(opts, temporal.OffsetReject); err != nil {
				return nil, err
			}
			if _, err = r.overflowOption(opts); err != nil {
				return nil, err
			}
			return &tZoned{ns: new(big.Int).Set(z.ns), tz: z.tz}, nil
		}
		return r.zonedFromFields(item, options)
	}
	if item.Type() != vm.TypeString {
		return nil, r.typeErr("a ZonedDateTime must be an object or a string")
	}
	p, err := temporal.ParseZonedDateTimeString(item.ToString())
	if err != nil {
		return nil, r.err(err)
	}
	tz, err := temporal.ParseTimeZone(p.TimeZone)
	if err != nil {
		return nil, r.err(err)
	}
	if p.Calendar != "" {
		if _, err := r.canonicalCalendar(p.Calendar); err != nil {
			return nil, err
		}
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return nil, err
	}
	dis, err := r.disambiguationOption(opts)
	if err != nil {
		return nil, err
	}
	offOpt, err := r.offsetOption(opts, temporal.OffsetReject)
	if err != nil {
		return nil, err
	}
	if _, err = r.overflowOption(opts); err != nil {
		return nil, err
	}
	var ns *big.Int
	if !p.HasTime {
		ns, err = r.startOfDay(tz, p.Date)
	} else {
		behaviour, off := offsetWall, int64(0)
		switch {
		case p.UTC:
			behaviour = offsetExact
		case p.HasOffset:
			behaviour, off = offsetOptionGiven, p.OffsetNs
		}
		ns, err = r.interpretOffset(temporal.DateTime{Date: p.Date, Time: p.Time}, behaviour, off, tz, dis, offOpt, !p.OffsetHasSubMinute)
	}
	if err != nil {
		return nil, err
	}
	return &tZoned{ns: ns, tz: tz}, nil
}

// zonedFromFields is the property-bag branch of ToTemporalZonedDateTime.
func (r *temporalRealm) zonedFromFields(item, options vm.Value) (*tZoned, error) {
	if err := r.calendarOfItem(item); err != nil {
		return nil, err
	}
	f, err := r.prepareFields(item, append([]string{fTimeZone}, zonedFieldNames...), []string{fTimeZone}, false)
	if err != nil {
		return nil, err
	}
	tz, err := r.toTemporalTimeZone(f.TimeZone)
	if err != nil {
		return nil, err
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return nil, err
	}
	dis, err := r.disambiguationOption(opts)
	if err != nil {
		return nil, err
	}
	offOpt, err := r.offsetOption(opts, temporal.OffsetReject)
	if err != nil {
		return nil, err
	}
	overflow, err := r.overflowOption(opts)
	if err != nil {
		return nil, err
	}
	dt, err := r.dateTimeFromFields(f, overflow)
	if err != nil {
		return nil, err
	}
	behaviour, off := offsetWall, int64(0)
	if f.Offset != nil {
		behaviour = offsetOptionGiven
		if off, _, err = temporal.ParseDateTimeUTCOffset(*f.Offset); err != nil {
			return nil, r.err(err)
		}
	}
	ns, err := r.interpretOffset(dt, behaviour, off, tz, dis, offOpt, false)
	if err != nil {
		return nil, err
	}
	return &tZoned{ns: ns, tz: tz}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// calendarOfItem is GetTemporalCalendarIdentifierWithISODefault for a
// property bag: only ISO 8601 exists, so it just validates the calendar.
func (r *temporalRealm) calendarOfItem(item vm.Value) error {
	if hasCalendarSlot(item) {
		return nil
	}
	c, err := r.vm.GetProperty(item, "calendar")
	if err != nil || c.IsUndefined() {
		return err
	}
	_, err = r.toCalendarIdentifier(c)
	return err
}

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

// toCalendarIdentifier is ToTemporalCalendarIdentifier.
func (r *temporalRealm) toCalendarIdentifier(v vm.Value) (string, error) {
	if hasCalendarSlot(v) {
		return "iso8601", nil
	}
	if v.Type() != vm.TypeString {
		return "", r.typeErr("calendar must be a string")
	}
	s := v.ToString()
	if c, ok := temporal.CanonicalizeCalendarIdentifier(s); ok {
		return c, nil
	}
	// ParseTemporalCalendarString: any ISO string, whose calendar annotation counts.
	var p *temporal.Parsed
	var err error
	for _, parse := range []func(string) (*temporal.Parsed, error){
		temporal.ParseZonedDateTimeString, temporal.ParsePlainDateTimeString, temporal.ParseInstantString,
		temporal.ParsePlainMonthDayString, temporal.ParsePlainYearMonthString, temporal.ParsePlainTimeString,
	} {
		if p, err = parse(s); err == nil {
			break
		}
	}
	if err != nil {
		return "", r.rangeErr("invalid calendar " + s)
	}
	if p.Calendar == "" {
		return "iso8601", nil
	}
	return r.canonicalCalendar(p.Calendar)
}

// dateTimeFromFields is InterpretTemporalDateTimeFields for the ISO calendar.
func (r *temporalRealm) dateTimeFromFields(f *tFields, overflow temporal.Overflow) (temporal.DateTime, error) {
	if f.Year == nil || f.Day == nil || (f.Month == nil && f.MonthCode == nil) {
		return temporal.DateTime{}, r.typeErr("missing required date fields")
	}
	month, _, err := r.monthFromFields(f)
	if err != nil {
		return temporal.DateTime{}, err
	}
	d, err := temporal.RegulateISODate(*f.Year, month, *f.Day, overflow)
	if err != nil {
		return temporal.DateTime{}, r.err(err)
	}
	or0 := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	t, err := temporal.RegulateTime(or0(f.Hour), or0(f.Minute), or0(f.Second), or0(f.Millisecond), or0(f.Microsecond), or0(f.Nanosecond), overflow)
	if err != nil {
		return temporal.DateTime{}, r.err(err)
	}
	return temporal.DateTime{Date: d, Time: t}, nil
}

type offsetBehaviour int

const (
	offsetWall offsetBehaviour = iota
	offsetExact
	offsetOptionGiven
)

// epochNsFor is GetEpochNanosecondsFor.
func (r *temporalRealm) epochNsFor(tz temporal.TimeZone, dt temporal.DateTime, d temporal.Disambiguation) (*big.Int, error) {
	if !temporal.ISODateTimeWithinLimits(dt) {
		return nil, r.rangeErr("date-time out of range")
	}
	ns, err := tz.EpochNanosecondsFor(dt, d)
	return ns, r.err(err)
}

// interpretOffset is InterpretISODateTimeOffset.
func (r *temporalRealm) interpretOffset(dt temporal.DateTime, beh offsetBehaviour, offsetNs int64, tz temporal.TimeZone, dis temporal.Disambiguation, opt temporal.OffsetOption, matchMinutes bool) (*big.Int, error) {
	if beh == offsetWall || opt == temporal.OffsetIgnore {
		return r.epochNsFor(tz, dt, dis)
	}
	if !temporal.ISODateTimeWithinLimits(dt) {
		return nil, r.rangeErr("date-time out of range")
	}
	utc := temporal.GetUTCEpochNanoseconds(dt)
	if beh == offsetExact || opt == temporal.OffsetUse {
		ns := utc.Sub(utc, big.NewInt(offsetNs))
		if !temporal.InstantWithinLimits(ns) {
			return nil, r.rangeErr("epoch nanoseconds out of range")
		}
		return ns, nil
	}
	// CheckISODaysRange
	if days := dt.Date.EpochDays(); days > 1e8 || days < -1e8 {
		return nil, r.rangeErr("date out of range")
	}
	for _, cand := range tz.PossibleEpochNanoseconds(dt) {
		if !temporal.InstantWithinLimits(cand) {
			return nil, r.rangeErr("epoch nanoseconds out of range")
		}
		off := new(big.Int).Sub(utc, cand).Int64()
		if off == offsetNs {
			return cand, nil
		}
		if matchMinutes {
			rounded := temporal.RoundNumberToIncrement(big.NewInt(off), big.NewInt(60e9), temporal.RoundHalfExpand)
			if rounded.Int64() == offsetNs {
				return cand, nil
			}
		}
	}
	if opt == temporal.OffsetReject {
		return nil, r.rangeErr("offset is invalid for the time zone at that date-time")
	}
	return r.epochNsFor(tz, dt, dis)
}

// startOfDay is GetStartOfDay: the first instant of a calendar day, which
// is the zone transition itself when midnight is skipped.
func (r *temporalRealm) startOfDay(tz temporal.TimeZone, d temporal.Date) (*big.Int, error) {
	mid := temporal.DateTime{Date: d}
	if !temporal.ISODateTimeWithinLimits(mid) {
		return nil, r.rangeErr("date out of range")
	}
	var ns *big.Int
	if possible := tz.PossibleEpochNanoseconds(mid); len(possible) > 0 {
		ns = possible[0]
	} else {
		before, err := tz.EpochNanosecondsFor(mid, temporal.DisambiguateEarlier)
		if err != nil {
			return nil, r.err(err)
		}
		var ok bool
		if ns, ok = tz.NextTransition(before); !ok {
			return nil, r.rangeErr("cannot determine the start of the day")
		}
	}
	if !temporal.InstantWithinLimits(ns) {
		return nil, r.rangeErr("epoch nanoseconds out of range")
	}
	return ns, nil
}

func (r *temporalRealm) newZoned(ns *big.Int, tz temporal.TimeZone) (vm.Value, error) {
	return r.createZonedDateTime(ns, tz)
}

func bigValue(n *big.Int) vm.Value { return vm.NewBigInt(new(big.Int).Set(n)) }

func intValue(n int) vm.Value { return vm.NumberValue(float64(n)) }

// ---------------------------------------------------------------------------
// Installation
// ---------------------------------------------------------------------------

func installZonedDateTime(r *temporalRealm) error {
	ctor, proto := r.newClass("ZonedDateTime", 2, func(args []vm.Value, protoVal vm.Value) (vm.Value, error) {
		ns, err := r.toBigInt(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		if !temporal.InstantWithinLimits(ns) {
			return vm.Undefined, r.rangeErr("epoch nanoseconds out of range")
		}
		tzArg := argAt(args, 1)
		if tzArg.Type() != vm.TypeString {
			return vm.Undefined, r.typeErr("time zone must be a string")
		}
		tz, err := temporal.ParseTimeZone(tzArg.ToString())
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		if cal := argAt(args, 2); !cal.IsUndefined() {
			if cal.Type() != vm.TypeString {
				return vm.Undefined, r.typeErr("calendar must be a string")
			}
			if _, err := r.canonicalCalendar(cal.ToString()); err != nil {
				return vm.Undefined, err
			}
		}
		return r.wrapWithProto(protoVal, &tZoned{ns: ns, tz: tz}), nil
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		z, err := r.toTemporalZonedDateTime(argAt(args, 0), argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		return r.newZoned(z.ns, z.tz)
	})
	r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
		a, err := r.toTemporalZonedDateTime(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		b, err := r.toTemporalZonedDateTime(argAt(args, 1), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return intValue(a.ns.Cmp(b.ns)), nil
	})

	r.installZonedGetters(proto)
	r.installZonedMethods(proto)
	return nil
}

func (r *temporalRealm) installZonedGetters(proto *vm.PlainObject) {
	// local reads a field of the wall-clock date-time.
	local := func(name string, f func(dt temporal.DateTime) vm.Value) {
		r.getter(proto, name, func(this vm.Value) (vm.Value, error) {
			z, err := thisSlots[tZoned](r, "ZonedDateTime")
			if err != nil {
				return vm.Undefined, err
			}
			return f(z.tz.LocalDateTime(z.ns)), nil
		})
	}
	exact := func(name string, f func(z *tZoned) vm.Value) {
		r.getter(proto, name, func(this vm.Value) (vm.Value, error) {
			z, err := thisSlots[tZoned](r, "ZonedDateTime")
			if err != nil {
				return vm.Undefined, err
			}
			return f(z), nil
		})
	}
	exact("calendarId", func(z *tZoned) vm.Value { return vm.NewString("iso8601") })
	exact("timeZoneId", func(z *tZoned) vm.Value { return vm.NewString(z.tz.ID()) })
	local("era", func(dt temporal.DateTime) vm.Value { return vm.Undefined })
	local("eraYear", func(dt temporal.DateTime) vm.Value { return vm.Undefined })
	local("year", func(dt temporal.DateTime) vm.Value { return intValue(dt.Date.Year) })
	local("month", func(dt temporal.DateTime) vm.Value { return intValue(dt.Date.Month) })
	local("monthCode", func(dt temporal.DateTime) vm.Value { return vm.NewString(fmt.Sprintf("M%02d", dt.Date.Month)) })
	local("day", func(dt temporal.DateTime) vm.Value { return intValue(dt.Date.Day) })
	local("hour", func(dt temporal.DateTime) vm.Value { return intValue(dt.Time.Hour) })
	local("minute", func(dt temporal.DateTime) vm.Value { return intValue(dt.Time.Minute) })
	local("second", func(dt temporal.DateTime) vm.Value { return intValue(dt.Time.Second) })
	local("millisecond", func(dt temporal.DateTime) vm.Value { return intValue(dt.Time.Millisecond) })
	local("microsecond", func(dt temporal.DateTime) vm.Value { return intValue(dt.Time.Microsecond) })
	local("nanosecond", func(dt temporal.DateTime) vm.Value { return intValue(dt.Time.Nanosecond) })
	exact("epochMilliseconds", func(z *tZoned) vm.Value {
		ms := new(big.Int).Div(z.ns, big.NewInt(1e6))
		return vm.NumberValue(float64(ms.Int64()))
	})
	exact("epochNanoseconds", func(z *tZoned) vm.Value { return bigValue(z.ns) })
	local("dayOfWeek", func(dt temporal.DateTime) vm.Value { return intValue(dt.Date.DayOfWeek()) })
	local("dayOfYear", func(dt temporal.DateTime) vm.Value { return intValue(dt.Date.DayOfYear()) })
	local("weekOfYear", func(dt temporal.DateTime) vm.Value {
		w, _ := dt.Date.WeekOfYear()
		return intValue(w)
	})
	local("yearOfWeek", func(dt temporal.DateTime) vm.Value {
		_, y := dt.Date.WeekOfYear()
		return intValue(y)
	})
	local("daysInWeek", func(dt temporal.DateTime) vm.Value { return intValue(7) })
	local("daysInMonth", func(dt temporal.DateTime) vm.Value {
		return intValue(temporal.DaysInMonth(dt.Date.Year, dt.Date.Month))
	})
	local("daysInYear", func(dt temporal.DateTime) vm.Value { return intValue(temporal.DaysInYear(dt.Date.Year)) })
	local("monthsInYear", func(dt temporal.DateTime) vm.Value { return intValue(12) })
	local("inLeapYear", func(dt temporal.DateTime) vm.Value { return vm.BooleanValue(temporal.IsLeapYear(dt.Date.Year)) })
	exact("offsetNanoseconds", func(z *tZoned) vm.Value {
		return vm.NumberValue(float64(z.tz.OffsetNanosecondsAt(z.ns)))
	})
	exact("offset", func(z *tZoned) vm.Value {
		return vm.NewString(temporal.FormatOffset(z.tz.OffsetNanosecondsAt(z.ns), false))
	})
	r.getter(proto, "hoursInDay", func(this vm.Value) (vm.Value, error) {
		z, err := thisSlots[tZoned](r, "ZonedDateTime")
		if err != nil {
			return vm.Undefined, err
		}
		today := z.tz.LocalDateTime(z.ns).Date
		start, err := r.startOfDay(z.tz, today)
		if err != nil {
			return vm.Undefined, err
		}
		tomorrow := temporal.BalanceISODate(today.Year, today.Month, int64(today.Day)+1)
		end, err := r.startOfDay(z.tz, tomorrow)
		if err != nil {
			return vm.Undefined, err
		}
		hours, _ := new(big.Rat).SetFrac(new(big.Int).Sub(end, start), big.NewInt(3600e9)).Float64()
		return vm.NumberValue(hours), nil
	})
}

func (r *temporalRealm) installZonedMethods(proto *vm.PlainObject) {
	// zm defines a method whose receiver has been brand-checked.
	zm := func(name string, length int, fn func(z *tZoned, args []vm.Value) (vm.Value, error)) {
		r.method(proto, name, length, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			z, err := thisSlots[tZoned](r, "ZonedDateTime")
			if err != nil {
				return vm.Undefined, err
			}
			return fn(z, args)
		})
	}

	zm("with", 1, r.zonedWith)
	zm("withPlainTime", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		date := z.tz.LocalDateTime(z.ns).Date
		var ns *big.Int
		var err error
		if arg := argAt(args, 0); arg.IsUndefined() {
			ns, err = r.startOfDay(z.tz, date)
		} else {
			var t temporal.Time
			if t, err = r.toTemporalTime(arg, vm.Undefined); err == nil {
				ns, err = r.epochNsFor(z.tz, temporal.DateTime{Date: date, Time: t}, temporal.DisambiguateCompatible)
			}
		}
		if err != nil {
			return vm.Undefined, err
		}
		return r.newZoned(ns, z.tz)
	})
	zm("withTimeZone", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		tz, err := r.toTemporalTimeZone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.newZoned(z.ns, tz)
	})
	zm("withCalendar", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		if _, err := r.toCalendarIdentifier(argAt(args, 0)); err != nil {
			return vm.Undefined, err
		}
		return r.newZoned(z.ns, z.tz)
	})
	zm("add", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) { return r.zonedAdd(z, args, 1) })
	zm("subtract", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) { return r.zonedAdd(z, args, -1) })
	zm("until", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) { return r.zonedDifference(z, args, false) })
	zm("since", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) { return r.zonedDifference(z, args, true) })
	zm("round", 1, r.zonedRound)
	zm("equals", 1, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		o, err := r.toTemporalZonedDateTime(argAt(args, 0), vm.Undefined)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(z.ns.Cmp(o.ns) == 0 && z.tz.Equal(o.tz)), nil
	})
	zm("startOfDay", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		ns, err := r.startOfDay(z.tz, z.tz.LocalDateTime(z.ns).Date)
		if err != nil {
			return vm.Undefined, err
		}
		return r.newZoned(ns, z.tz)
	})
	zm("getTimeZoneTransition", 1, r.zonedTransition)
	zm("toString", 0, r.zonedToString)
	zm("toLocaleString", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		return vm.NewString(zonedString(z, -1, "auto", "auto", "auto", 1, temporal.UnitNanosecond, temporal.RoundTrunc)), nil
	})
	zm("toJSON", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		return vm.NewString(zonedString(z, -1, "auto", "auto", "auto", 1, temporal.UnitNanosecond, temporal.RoundTrunc)), nil
	})
	r.method(proto, "valueOf", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("Temporal.ZonedDateTime cannot be converted to a primitive; use compare() or equals()")
	})
	zm("toInstant", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) { return r.createInstant(new(big.Int).Set(z.ns)) })
	zm("toPlainDate", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		return r.createPlainDate(z.tz.LocalDateTime(z.ns).Date)
	})
	zm("toPlainTime", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		return r.createPlainTime(z.tz.LocalDateTime(z.ns).Time)
	})
	zm("toPlainDateTime", 0, func(z *tZoned, args []vm.Value) (vm.Value, error) {
		return r.createPlainDateTime(z.tz.LocalDateTime(z.ns))
	})
}

// zonedWith is ZonedDateTime.prototype.with.
func (r *temporalRealm) zonedWith(z *tZoned, args []vm.Value) (vm.Value, error) {
	item := argAt(args, 0)
	// IsPartialTemporalObject
	if !isObjectValue(item) || hasTemporalSlots(item) {
		return vm.Undefined, r.typeErr("with() needs an object with date-time fields")
	}
	for _, name := range []string{"calendar", "timeZone"} {
		v, err := r.vm.GetProperty(item, name)
		if err != nil {
			return vm.Undefined, err
		}
		if !v.IsUndefined() {
			return vm.Undefined, r.typeErr("with() does not accept a " + name + " property")
		}
	}
	partial, err := r.prepareFields(item, zonedFieldNames, nil, true)
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
	offOpt, err := r.offsetOption(opts, temporal.OffsetPrefer)
	if err != nil {
		return vm.Undefined, err
	}
	overflow, err := r.overflowOption(opts)
	if err != nil {
		return vm.Undefined, err
	}

	// CalendarMergeFields: a new month or monthCode replaces both old ones.
	cur := z.tz.LocalDateTime(z.ns)
	merged := &tFields{
		Year: &cur.Date.Year, Month: &cur.Date.Month, Day: &cur.Date.Day,
		Hour: &cur.Time.Hour, Minute: &cur.Time.Minute, Second: &cur.Time.Second,
		Millisecond: &cur.Time.Millisecond, Microsecond: &cur.Time.Microsecond, Nanosecond: &cur.Time.Nanosecond,
	}
	pick := func(dst **int, src *int) {
		if src != nil {
			*dst = src
		}
	}
	pick(&merged.Year, partial.Year)
	pick(&merged.Day, partial.Day)
	pick(&merged.Hour, partial.Hour)
	pick(&merged.Minute, partial.Minute)
	pick(&merged.Second, partial.Second)
	pick(&merged.Millisecond, partial.Millisecond)
	pick(&merged.Microsecond, partial.Microsecond)
	pick(&merged.Nanosecond, partial.Nanosecond)
	if partial.Month != nil || partial.MonthCode != nil {
		merged.Month, merged.MonthCode = partial.Month, partial.MonthCode
	}
	dt, err := r.dateTimeFromFields(merged, overflow)
	if err != nil {
		return vm.Undefined, err
	}
	off := z.tz.OffsetNanosecondsAt(z.ns)
	if partial.Offset != nil {
		if off, _, err = temporal.ParseDateTimeUTCOffset(*partial.Offset); err != nil {
			return vm.Undefined, r.err(err)
		}
	}
	ns, err := r.interpretOffset(dt, offsetOptionGiven, off, z.tz, dis, offOpt, false)
	if err != nil {
		return vm.Undefined, err
	}
	return r.newZoned(ns, z.tz)
}

func hasTemporalSlots(v vm.Value) bool {
	if hasCalendarSlot(v) {
		return true
	}
	if _, ok := slotsOf[tPlainTime](v); ok {
		return true
	}
	if _, ok := slotsOf[tInstant](v); ok {
		return true
	}
	_, ok := slotsOf[tDuration](v)
	return ok
}

// zonedAdd is AddDurationToZonedDateTime; sign is -1 for subtract.
func (r *temporalRealm) zonedAdd(z *tZoned, args []vm.Value, sign int) (vm.Value, error) {
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
	ns, err := temporal.AddZonedDateTime(z.ns, z.tz, in.Date, in.Time, overflow)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	return r.newZoned(ns, z.tz)
}

// zonedDifference is DifferenceTemporalZonedDateTime.
func (r *temporalRealm) zonedDifference(z *tZoned, args []vm.Value, since bool) (vm.Value, error) {
	o, err := r.toTemporalZonedDateTime(argAt(args, 0), vm.Undefined)
	if err != nil {
		return vm.Undefined, err
	}
	opts, err := r.getOptionsObject(argAt(args, 1))
	if err != nil {
		return vm.Undefined, err
	}
	s, err := r.differenceSettings(since, opts, unitsDateTime, nil, temporal.UnitNanosecond, temporal.UnitHour)
	if err != nil {
		return vm.Undefined, err
	}
	if !s.largest.IsTimeUnit() && !z.tz.Equal(o.tz) {
		return vm.Undefined, r.rangeErr("time zones must match to compute a difference in calendar units")
	}
	if z.ns.Cmp(o.ns) == 0 {
		return r.createDuration(temporal.Duration{})
	}
	in, err := temporal.DifferenceZonedDateTimeWithRounding(z.ns, o.ns, z.tz, s.largest, s.increment, s.smallest, s.mode)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	balance := s.largest
	if !balance.IsTimeUnit() {
		balance = temporal.UnitHour
	}
	d, err := in.ToDuration(balance)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	if since {
		d = d.Negated()
	}
	return r.createDuration(d)
}

// zonedRound is ZonedDateTime.prototype.round.
func (r *temporalRealm) zonedRound(z *tZoned, args []vm.Value) (vm.Value, error) {
	roundTo := argAt(args, 0)
	if roundTo.IsUndefined() {
		return vm.Undefined, r.typeErr("round() requires an argument")
	}
	var (
		increment = int64(1)
		mode      = temporal.RoundHalfExpand
		unitName  string
		present   bool
		err       error
	)
	if roundTo.Type() == vm.TypeString {
		unitName, present = roundTo.ToString(), true
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
		if unitName, present, err = r.rawUnitOption(opts, "smallestUnit"); err != nil {
			return vm.Undefined, err
		}
	}
	if !present {
		return vm.Undefined, r.rangeErr("the smallestUnit option is required")
	}
	unit, ok := unitNames[unitName]
	if !ok || !(unit.IsTimeUnit() || unit == temporal.UnitDay) {
		return vm.Undefined, r.rangeErr(unitName + " is not a valid value for option smallestUnit")
	}
	if unit == temporal.UnitDay {
		err = temporal.ValidateRoundingIncrement(increment, 1, true)
	} else {
		max, _ := temporal.MaximumTemporalDurationRoundingIncrement(unit)
		err = temporal.ValidateRoundingIncrement(increment, max, false)
	}
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	if unit == temporal.UnitNanosecond && increment == 1 {
		return r.newZoned(z.ns, z.tz)
	}
	cur := z.tz.LocalDateTime(z.ns)
	var ns *big.Int
	if unit == temporal.UnitDay {
		start, err := r.startOfDay(z.tz, cur.Date)
		if err != nil {
			return vm.Undefined, err
		}
		next := temporal.BalanceISODate(cur.Date.Year, cur.Date.Month, int64(cur.Date.Day)+1)
		end, err := r.startOfDay(z.tz, next)
		if err != nil {
			return vm.Undefined, err
		}
		length := new(big.Int).Sub(end, start)
		progress := new(big.Int).Sub(z.ns, start)
		ns = start.Add(start, temporal.RoundNumberToIncrement(progress, length, mode))
	} else {
		rounded, err := temporal.RoundISODateTime(cur, increment, unit, mode)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		off := z.tz.OffsetNanosecondsAt(z.ns)
		if ns, err = r.interpretOffset(rounded, offsetOptionGiven, off, z.tz, temporal.DisambiguateCompatible, temporal.OffsetPrefer, false); err != nil {
			return vm.Undefined, err
		}
	}
	return r.newZoned(ns, z.tz)
}

// zonedTransition is getTimeZoneTransition.
func (r *temporalRealm) zonedTransition(z *tZoned, args []vm.Value) (vm.Value, error) {
	param := argAt(args, 0)
	if param.IsUndefined() {
		return vm.Undefined, r.typeErr("getTimeZoneTransition() requires an argument")
	}
	var direction string
	if param.Type() == vm.TypeString {
		direction = param.ToString()
		if direction != "next" && direction != "previous" {
			return vm.Undefined, r.rangeErr(direction + " is not a valid value for option direction")
		}
	} else {
		opts, err := r.getOptionsObject(param)
		if err != nil {
			return vm.Undefined, err
		}
		var present bool
		if direction, present, err = r.enumOption(opts, "direction", []string{"next", "previous"}, ""); err != nil {
			return vm.Undefined, err
		}
		if !present {
			return vm.Undefined, r.rangeErr("the direction option is required")
		}
	}
	var t *big.Int
	var ok bool
	if direction == "next" {
		t, ok = z.tz.NextTransition(z.ns)
	} else {
		t, ok = z.tz.PreviousTransition(z.ns)
	}
	if !ok {
		return vm.Null, nil
	}
	return r.newZoned(t, z.tz)
}

// zonedToString is ZonedDateTime.prototype.toString.
func (r *temporalRealm) zonedToString(z *tZoned, args []vm.Value) (vm.Value, error) {
	opts, err := r.getOptionsObject(argAt(args, 0))
	if err != nil {
		return vm.Undefined, err
	}
	showCalendar, err := r.calendarNameOption(opts)
	if err != nil {
		return vm.Undefined, err
	}
	digits, err := r.fractionalSecondDigitsOption(opts)
	if err != nil {
		return vm.Undefined, err
	}
	showOffset, _, err := r.enumOption(opts, "offset", []string{"auto", "never"}, "auto")
	if err != nil {
		return vm.Undefined, err
	}
	mode, err := r.roundingModeOption(opts, temporal.RoundTrunc)
	if err != nil {
		return vm.Undefined, err
	}
	smallestRaw, present, err := r.rawUnitOption(opts, "smallestUnit")
	if err != nil {
		return vm.Undefined, err
	}
	showZone, _, err := r.enumOption(opts, "timeZoneName", []string{"auto", "never", "critical"}, "auto")
	if err != nil {
		return vm.Undefined, err
	}
	smallest, present, err := r.validateUnit("smallestUnit", smallestRaw, present, unitsTime, false)
	if err != nil {
		return vm.Undefined, err
	}
	if present && smallest == temporal.UnitHour {
		return vm.Undefined, r.rangeErr("smallestUnit hour is not valid for toString")
	}
	precision, unit, increment := secondsStringPrecision(smallest, present, digits)
	return vm.NewString(zonedString(z, precision, showCalendar, showZone, showOffset, increment, unit, mode)), nil
}

// roundAsIfPositive is RoundNumberToIncrementAsIfPositive: trunc and expand
// round toward the Big Bang rather than toward zero. Shifting x by an even
// multiple of the increment makes it positive without changing the result.
func roundAsIfPositive(x, increment *big.Int, mode temporal.RoundingMode) *big.Int {
	shift := new(big.Int).Quo(new(big.Int).Abs(x), increment)
	shift.Add(shift, big.NewInt(1)).Lsh(shift, 1).Mul(shift, increment)
	r := temporal.RoundNumberToIncrement(new(big.Int).Add(x, shift), increment, mode)
	return r.Sub(r, shift)
}

// zonedString is TemporalZonedDateTimeToString.
func zonedString(z *tZoned, precision int, showCalendar, showZone, showOffset string, increment int64, unit temporal.Unit, mode temporal.RoundingMode) string {
	ns := z.ns
	if !(unit == temporal.UnitNanosecond && increment == 1) {
		ns = roundAsIfPositive(ns, big.NewInt(increment*unit.NanosecondsPerUnit()), mode)
	}
	out := temporal.FormatDateTime(z.tz.LocalDateTime(ns), precision)
	if showOffset != "never" {
		out += temporal.FormatOffset(z.tz.OffsetNanosecondsAt(ns), true)
	}
	switch showZone {
	case "auto":
		out += "[" + z.tz.ID() + "]"
	case "critical":
		out += "[!" + z.tz.ID() + "]"
	}
	switch showCalendar {
	case "always":
		out += "[u-ca=iso8601]"
	case "critical":
		out += "[!u-ca=iso8601]"
	}
	return out
}

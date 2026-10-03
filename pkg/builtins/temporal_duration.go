package builtins

import (
	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.Duration.

func init() { registerTemporalInstaller(installDuration) }

// createDuration is CreateTemporalDuration.
func (r *temporalRealm) createDuration(d temporal.Duration) (vm.Value, error) {
	if !temporal.IsValidDuration(d) {
		return vm.Undefined, r.rangeErr("invalid duration")
	}
	return r.wrap("Duration", &tDuration{d: d}), nil
}

// durationFieldSetters maps the field names, in the spec's reading order
// (alphabetical), to the Duration field they set.
var durationFieldSetters = [10]func(*temporal.Duration, float64){
	func(d *temporal.Duration, v float64) { d.Days = v },
	func(d *temporal.Duration, v float64) { d.Hours = v },
	func(d *temporal.Duration, v float64) { d.Microseconds = v },
	func(d *temporal.Duration, v float64) { d.Milliseconds = v },
	func(d *temporal.Duration, v float64) { d.Minutes = v },
	func(d *temporal.Duration, v float64) { d.Months = v },
	func(d *temporal.Duration, v float64) { d.Nanoseconds = v },
	func(d *temporal.Duration, v float64) { d.Seconds = v },
	func(d *temporal.Duration, v float64) { d.Weeks = v },
	func(d *temporal.Duration, v float64) { d.Years = v },
}

// durationWith is Duration.prototype.with: the fields of item that are not
// undefined replace the receiver's.
func (r *temporalRealm) durationWith(base temporal.Duration, item vm.Value) (temporal.Duration, error) {
	if !isObjectValue(item) {
		return temporal.Duration{}, r.typeErr("the argument must be an object")
	}
	any := false
	d := base
	for i, name := range durationFieldNames {
		v, err := r.vm.GetProperty(item, name)
		if err != nil {
			return temporal.Duration{}, err
		}
		if v.IsUndefined() {
			continue
		}
		any = true
		n, err := r.toIntegerIfIntegral(v)
		if err != nil {
			return temporal.Duration{}, err
		}
		durationFieldSetters[i](&d, n)
	}
	if !any {
		return temporal.Duration{}, r.typeErr("the object has no Duration fields")
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// relativeTo
// ---------------------------------------------------------------------------

// relCalendarIdentifier is ToTemporalCalendarIdentifier for a relativeTo
// bag's calendar property: only iso8601 exists.
func (r *temporalRealm) relCalendarIdentifier(v vm.Value) error {
	if isObjectValue(v) {
		if _, ok := slotsOf[tPlainDate](v); ok {
			return nil
		}
		if _, ok := slotsOf[tPlainDateTime](v); ok {
			return nil
		}
		if _, ok := slotsOf[tZoned](v); ok {
			return nil
		}
		if _, ok := slotsOf[tPlainYearMonth](v); ok {
			return nil
		}
		if _, ok := slotsOf[tPlainMonthDay](v); ok {
			return nil
		}
	}
	if v.Type() != vm.TypeString {
		return r.typeErr("calendar must be a string")
	}
	s := v.ToString()
	if _, err := r.canonicalCalendar(s); err == nil {
		return nil
	}
	// A calendar may also be given as an ISO string with an annotation.
	for _, parse := range []func(string) (*temporal.Parsed, error){temporal.ParsePlainDateTimeString, temporal.ParsePlainTimeString, temporal.ParsePlainYearMonthString, temporal.ParsePlainMonthDayString} {
		if p, err := parse(s); err == nil {
			if p.Calendar == "" {
				return nil
			}
			_, cerr := r.canonicalCalendar(p.Calendar)
			return cerr
		}
	}
	return r.rangeErr("unsupported calendar " + s)
}

func relFromZoned(s *tZoned) *temporal.RelativeTo {
	local := s.tz.LocalDateTime(s.ns)
	return &temporal.RelativeTo{Date: local.Date, Time: local.Time, Zone: s.tz, EpochNs: s.ns}
}

func (r *temporalRealm) relFromDate(d temporal.Date) (*temporal.RelativeTo, error) {
	if !temporal.ISODateWithinLimits(d) {
		return nil, r.rangeErr("date out of range")
	}
	return &temporal.RelativeTo{Date: d}, nil
}

// relativeToOption is GetTemporalRelativeToOption.
func (r *temporalRealm) relativeToOption(options vm.Value) (*temporal.RelativeTo, error) {
	value, err := r.option(options, "relativeTo")
	if err != nil || value.IsUndefined() {
		return nil, err
	}
	if isObjectValue(value) {
		if s, ok := slotsOf[tZoned](value); ok {
			return relFromZoned(s), nil
		}
		if s, ok := slotsOf[tPlainDate](value); ok {
			return &temporal.RelativeTo{Date: s.date}, nil
		}
		if s, ok := slotsOf[tPlainDateTime](value); ok {
			return r.relFromDate(s.dt.Date)
		}
		cal, err := r.vm.GetProperty(value, "calendar")
		if err != nil {
			return nil, err
		}
		if !cal.IsUndefined() {
			if err := r.relCalendarIdentifier(cal); err != nil {
				return nil, err
			}
		}
		f, err := r.prepareFields(value, []string{fDay, fHour, fMicrosecond, fMillisecond, fMinute, fMonth, fMonthCode, fNanosecond, fOffset, fSecond, fTimeZone, fYear}, nil, false)
		if err != nil {
			return nil, err
		}
		date, tm, err := r.relativeFieldsToDateTime(f)
		if err != nil {
			return nil, err
		}
		if f.TimeZone.IsUndefined() {
			return r.relFromDate(date)
		}
		tz, err := r.toTemporalTimeZone(f.TimeZone)
		if err != nil {
			return nil, err
		}
		offsetNs, behaviour := int64(0), offsetWall
		if f.Offset != nil {
			offsetNs, _, _ = temporal.ParseDateTimeUTCOffset(*f.Offset)
			behaviour = offsetOptionGiven
		}
		dt := temporal.DateTime{Date: date, Time: tm}
		if !temporal.ISODateTimeWithinLimits(dt) {
			return nil, r.rangeErr("date-time out of range")
		}
		ns, err := r.interpretOffset(dt, behaviour, offsetNs, tz, temporal.DisambiguateCompatible, temporal.OffsetReject, false)
		if err != nil {
			return nil, err
		}
		return relFromZoned(&tZoned{ns: ns, tz: tz}), nil
	}
	if value.Type() != vm.TypeString {
		return nil, r.typeErr("relativeTo must be an object or a string")
	}
	p, err := temporal.ParseZonedDateTimeString(value.ToString())
	if err == nil {
		tz, terr := temporal.ParseTimeZone(p.TimeZone)
		if terr != nil {
			return nil, r.err(terr)
		}
		if p.Calendar != "" {
			if _, err := r.canonicalCalendar(p.Calendar); err != nil {
				return nil, err
			}
		}
		dt := temporal.DateTime{Date: p.Date, Time: p.Time}
		if !temporal.ISODateTimeWithinLimits(dt) {
			return nil, r.rangeErr("date-time out of range")
		}
		behaviour := offsetWall
		switch {
		case p.UTC:
			behaviour = offsetExact
		case p.HasOffset:
			behaviour = offsetOptionGiven
		}
		ns, err := r.interpretOffset(dt, behaviour, p.OffsetNs, tz, temporal.DisambiguateCompatible, temporal.OffsetReject, !p.OffsetHasSubMinute)
		if err != nil {
			return nil, err
		}
		return relFromZoned(&tZoned{ns: ns, tz: tz}), nil
	}
	p, perr := temporal.ParsePlainDateTimeString(value.ToString())
	if perr != nil {
		return nil, r.err(perr)
	}
	if p.Calendar != "" {
		if _, err := r.canonicalCalendar(p.Calendar); err != nil {
			return nil, err
		}
	}
	return r.relFromDate(p.Date)
}

// relativeFieldsToDateTime resolves a relativeTo bag's date and time fields
// with "constrain" overflow.
func (r *temporalRealm) relativeFieldsToDateTime(f *tFields) (temporal.Date, temporal.Time, error) {
	if f.Year == nil {
		return temporal.Date{}, temporal.Time{}, r.typeErr("missing required property year")
	}
	month, ok, err := r.monthFromFields(f)
	if err != nil {
		return temporal.Date{}, temporal.Time{}, err
	}
	if !ok {
		return temporal.Date{}, temporal.Time{}, r.typeErr("missing required property month")
	}
	if f.Day == nil {
		return temporal.Date{}, temporal.Time{}, r.typeErr("missing required property day")
	}
	date, err := temporal.RegulateISODate(*f.Year, month, *f.Day, temporal.OverflowConstrain)
	if err != nil {
		return temporal.Date{}, temporal.Time{}, r.err(err)
	}
	get := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	tm, err := temporal.RegulateTime(get(f.Hour), get(f.Minute), get(f.Second), get(f.Millisecond), get(f.Microsecond), get(f.Nanosecond), temporal.OverflowConstrain)
	return date, tm, r.err(err)
}

// ---------------------------------------------------------------------------

func installDuration(r *temporalRealm) error {
	ctor, proto := r.newClass("Duration", 0, func(args []vm.Value, p vm.Value) (vm.Value, error) {
		var d temporal.Duration
		order := [10]func(*temporal.Duration, float64){
			func(d *temporal.Duration, v float64) { d.Years = v },
			func(d *temporal.Duration, v float64) { d.Months = v },
			func(d *temporal.Duration, v float64) { d.Weeks = v },
			func(d *temporal.Duration, v float64) { d.Days = v },
			func(d *temporal.Duration, v float64) { d.Hours = v },
			func(d *temporal.Duration, v float64) { d.Minutes = v },
			func(d *temporal.Duration, v float64) { d.Seconds = v },
			func(d *temporal.Duration, v float64) { d.Milliseconds = v },
			func(d *temporal.Duration, v float64) { d.Microseconds = v },
			func(d *temporal.Duration, v float64) { d.Nanoseconds = v },
		}
		for i, set := range order {
			a := argAt(args, i)
			if a.IsUndefined() {
				continue
			}
			n, err := r.toIntegerIfIntegral(a)
			if err != nil {
				return vm.Undefined, err
			}
			set(&d, n)
		}
		if !temporal.IsValidDuration(d) {
			return vm.Undefined, r.rangeErr("invalid duration")
		}
		return r.wrapWithProto(p, &tDuration{d: d}), nil
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		d, err := r.toTemporalDuration(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createDuration(d)
	})
	r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
		one, err := r.toTemporalDuration(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		two, err := r.toTemporalDuration(argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		opts, err := r.getOptionsObject(argAt(args, 2))
		if err != nil {
			return vm.Undefined, err
		}
		rel, err := r.relativeToOption(opts)
		if err != nil {
			return vm.Undefined, err
		}
		c, err := temporal.CompareDurations(one, two, rel)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		return vm.NumberValue(float64(c)), nil
	})

	numGetter := func(name string, get func(d temporal.Duration) float64) {
		r.getter(proto, name, func(this vm.Value) (vm.Value, error) {
			s, err := thisSlots[tDuration](r, "Duration")
			if err != nil {
				return vm.Undefined, err
			}
			return vm.NumberValue(get(s.d)), nil
		})
	}
	numGetter("years", func(d temporal.Duration) float64 { return d.Years })
	numGetter("months", func(d temporal.Duration) float64 { return d.Months })
	numGetter("weeks", func(d temporal.Duration) float64 { return d.Weeks })
	numGetter("days", func(d temporal.Duration) float64 { return d.Days })
	numGetter("hours", func(d temporal.Duration) float64 { return d.Hours })
	numGetter("minutes", func(d temporal.Duration) float64 { return d.Minutes })
	numGetter("seconds", func(d temporal.Duration) float64 { return d.Seconds })
	numGetter("milliseconds", func(d temporal.Duration) float64 { return d.Milliseconds })
	numGetter("microseconds", func(d temporal.Duration) float64 { return d.Microseconds })
	numGetter("nanoseconds", func(d temporal.Duration) float64 { return d.Nanoseconds })
	numGetter("sign", func(d temporal.Duration) float64 { return float64(temporal.DurationSign(d)) })
	r.getter(proto, "blank", func(this vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(temporal.DurationSign(s.d) == 0), nil
	})

	r.method(proto, "with", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		d, err := r.durationWith(s.d, argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createDuration(d)
	})
	r.method(proto, "negated", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		return r.createDuration(s.d.Negated())
	})
	r.method(proto, "abs", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		return r.createDuration(s.d.Abs())
	})
	addOrSubtract := func(name string, sign int) {
		r.method(proto, name, 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tDuration](r, "Duration")
			if err != nil {
				return vm.Undefined, err
			}
			other, err := r.toTemporalDuration(argAt(args, 0))
			if err != nil {
				return vm.Undefined, err
			}
			d, err := temporal.AddDurations(s.d, other, sign, nil)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			return r.createDuration(d)
		})
	}
	addOrSubtract("add", 1)
	addOrSubtract("subtract", -1)

	r.method(proto, "round", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		return r.durationRound(s.d, argAt(args, 0))
	})
	r.method(proto, "total", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		return r.durationTotal(s.d, argAt(args, 0))
	})

	toString := func(this vm.Value, options vm.Value) (vm.Value, error) {
		s, err := thisSlots[tDuration](r, "Duration")
		if err != nil {
			return vm.Undefined, err
		}
		return r.durationToString(s.d, options)
	}
	r.method(proto, "toString", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) { return toString(this, argAt(args, 0)) })
	r.method(proto, "toJSON", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) { return toString(this, vm.Undefined) })
	r.method(proto, "toLocaleString", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) { return toString(this, vm.Undefined) })
	r.method(proto, "valueOf", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("use compare() to compare Temporal.Duration")
	})
	return nil
}

// durationRound is Temporal.Duration.prototype.round.
func (r *temporalRealm) durationRound(d temporal.Duration, roundTo vm.Value) (vm.Value, error) {
	if roundTo.IsUndefined() {
		return vm.Undefined, r.typeErr("round requires a roundTo argument")
	}
	var (
		largestRaw, smallestRaw         string
		largestPresent, smallestPresent bool
		increment                       int64 = 1
		mode                                  = temporal.RoundHalfExpand
		rel                             *temporal.RelativeTo
	)
	if roundTo.Type() == vm.TypeString {
		smallestRaw, smallestPresent = roundTo.ToString(), true
	} else {
		opts, err := r.getOptionsObject(roundTo)
		if err != nil {
			return vm.Undefined, err
		}
		if largestRaw, largestPresent, err = r.rawUnitOption(opts, "largestUnit"); err != nil {
			return vm.Undefined, err
		}
		if rel, err = r.relativeToOption(opts); err != nil {
			return vm.Undefined, err
		}
		if increment, err = r.roundingIncrementOption(opts); err != nil {
			return vm.Undefined, err
		}
		if mode, err = r.roundingModeOption(opts, temporal.RoundHalfExpand); err != nil {
			return vm.Undefined, err
		}
		if smallestRaw, smallestPresent, err = r.rawUnitOption(opts, "smallestUnit"); err != nil {
			return vm.Undefined, err
		}
	}
	if !largestPresent && !smallestPresent {
		return vm.Undefined, r.rangeErr("at least one of largestUnit and smallestUnit is required")
	}
	largest, ok, err := r.validateUnit("largestUnit", largestRaw, largestPresent, unitsDateTime, true)
	if err != nil {
		return vm.Undefined, err
	}
	if !ok {
		largest = temporal.UnitAuto
	}
	smallest, ok, err := r.validateUnit("smallestUnit", smallestRaw, smallestPresent, unitsDateTime, false)
	if err != nil {
		return vm.Undefined, err
	}
	if !ok {
		smallest = temporal.UnitNanosecond
	}
	if largest == temporal.UnitAuto {
		largest = temporal.LargerOfTwoUnits(temporal.DefaultTemporalLargestUnit(d), smallest)
	}
	if temporal.LargerOfTwoUnits(largest, smallest) != largest {
		return vm.Undefined, r.rangeErr("largestUnit must be larger than smallestUnit")
	}
	if max, ok := temporal.MaximumTemporalDurationRoundingIncrement(smallest); ok {
		if err := temporal.ValidateRoundingIncrement(increment, max, false); err != nil {
			return vm.Undefined, r.err(err)
		}
	}
	if increment > 1 && largest != smallest && smallest.IsDateUnit() {
		return vm.Undefined, r.rangeErr("cannot round to an increment of " + smallest.String() + " while also balancing to " + largest.String())
	}
	res, err := temporal.RoundDuration(d, largest, increment, smallest, mode, rel)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	return r.createDuration(res)
}

// durationTotal is Temporal.Duration.prototype.total.
func (r *temporalRealm) durationTotal(d temporal.Duration, totalOf vm.Value) (vm.Value, error) {
	if totalOf.IsUndefined() {
		return vm.Undefined, r.typeErr("total requires a unit")
	}
	var raw string
	var present bool
	var rel *temporal.RelativeTo
	if totalOf.Type() == vm.TypeString {
		raw, present = totalOf.ToString(), true
	} else {
		opts, err := r.getOptionsObject(totalOf)
		if err != nil {
			return vm.Undefined, err
		}
		if rel, err = r.relativeToOption(opts); err != nil {
			return vm.Undefined, err
		}
		if raw, present, err = r.rawUnitOption(opts, "unit"); err != nil {
			return vm.Undefined, err
		}
	}
	if !present {
		return vm.Undefined, r.rangeErr("the unit option is required")
	}
	unit, _, err := r.validateUnit("unit", raw, present, unitsDateTime, false)
	if err != nil {
		return vm.Undefined, err
	}
	total, err := temporal.TotalDuration(d, unit, rel)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	f, _ := total.Float64()
	if f == 0 {
		f = 0
	}
	return vm.NumberValue(f), nil
}

// durationToString is Temporal.Duration.prototype.toString.
func (r *temporalRealm) durationToString(d temporal.Duration, options vm.Value) (vm.Value, error) {
	opts, err := r.getOptionsObject(options)
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
	smallest, _, err := r.validateUnit("smallestUnit", raw, present, unitsTime, false)
	if err != nil {
		return vm.Undefined, err
	}
	if present && (smallest == temporal.UnitHour || smallest == temporal.UnitMinute) {
		return vm.Undefined, r.rangeErr("smallestUnit " + smallest.String() + " is not allowed")
	}
	precision, unit, increment := secondsStringPrecision(smallest, present, digits)
	if unit != temporal.UnitNanosecond || increment != 1 {
		in := temporal.ToInternalDuration(d)
		rounded, err := temporal.RoundTimeDuration(in.Time, increment, unit, mode)
		if err != nil {
			return vm.Undefined, r.err(err)
		}
		largest := temporal.LargerOfTwoUnits(temporal.DefaultTemporalLargestUnit(d), temporal.UnitSecond)
		if d, err = (temporal.InternalDuration{Date: in.Date, Time: rounded}).ToDuration(largest); err != nil {
			return vm.Undefined, r.err(err)
		}
	}
	return vm.NewString(temporal.FormatDuration(d, precision)), nil
}

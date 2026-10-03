package builtins

import (
	"math"
	"math/big"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.Instant.

func init() { registerTemporalInstaller(installInstant) }

var bigNsPerMs = big.NewInt(1e6)

func validEpochNs(ns *big.Int) bool { return temporal.InstantWithinLimits(ns) }

// createInstant is CreateTemporalInstant.
func (r *temporalRealm) createInstant(ns *big.Int) (vm.Value, error) {
	if !validEpochNs(ns) {
		return vm.Undefined, r.rangeErr("epoch nanoseconds out of range")
	}
	return r.wrap("Instant", &tInstant{ns: ns}), nil
}

// instantFromParsed is the epoch time of a parsed instant string.
func (r *temporalRealm) instantFromParsed(p *temporal.Parsed) (*big.Int, error) {
	offset := int64(0)
	if p.HasOffset {
		offset = p.OffsetNs
	}
	ns := temporal.GetUTCEpochNanoseconds(temporal.DateTime{Date: p.Date, Time: p.Time})
	ns.Sub(ns, big.NewInt(offset))
	if !validEpochNs(ns) {
		return nil, r.rangeErr("instant out of range")
	}
	return ns, nil
}

// toTemporalInstant is ToTemporalInstant.
func (r *temporalRealm) toTemporalInstant(item vm.Value) (*big.Int, error) {
	if isObjectValue(item) {
		if s, ok := slotsOf[tInstant](item); ok {
			return s.ns, nil
		}
		if s, ok := slotsOf[tZoned](item); ok {
			return s.ns, nil
		}
		r.vm.EnterHelperCall()
		item = r.vm.ToPrimitive(item, "string")
		r.vm.ExitHelperCall()
		if r.vm.IsUnwinding() || r.vm.IsHandlerFound() {
			return nil, ErrVMUnwinding
		}
	}
	if item.Type() != vm.TypeString {
		return nil, r.typeErr("an instant must be an object or a string")
	}
	p, err := temporal.ParseInstantString(item.ToString())
	if err != nil {
		return nil, r.err(err)
	}
	return r.instantFromParsed(p)
}

// floorDivBig is floor(a / b) for b > 0.
func floorDivBig(a, b *big.Int) *big.Int {
	q, _ := new(big.Int).DivMod(a, b, new(big.Int))
	return q
}

// roundTemporalInstant is RoundTemporalInstant.
func roundTemporalInstant(ns *big.Int, increment int64, unit temporal.Unit, mode temporal.RoundingMode) *big.Int {
	return temporal.RoundNumberToIncrementAsIfPositive(ns, big.NewInt(increment*unit.NanosecondsPerUnit()), mode)
}

// addDurationToInstant is AddDurationToInstant: calendar units and days are
// a RangeError for an Instant.
func (r *temporalRealm) addDurationToInstant(sign int, ns *big.Int, durationLike vm.Value) (vm.Value, error) {
	d, err := r.toTemporalDuration(durationLike)
	if err != nil {
		return vm.Undefined, err
	}
	if d.Years != 0 || d.Months != 0 || d.Weeks != 0 || d.Days != 0 {
		return vm.Undefined, r.rangeErr("an Instant cannot be changed by years, months, weeks or days")
	}
	in := temporal.ToInternalDuration(d)
	t := in.Time
	if sign < 0 {
		t = new(big.Int).Neg(t)
	}
	res, err := temporal.AddInstant(ns, t)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	return r.createInstant(res)
}

// differenceInstant is DifferenceTemporalInstant.
func (r *temporalRealm) differenceInstant(since bool, ns *big.Int, otherLike, options vm.Value) (vm.Value, error) {
	other, err := r.toTemporalInstant(otherLike)
	if err != nil {
		return vm.Undefined, err
	}
	opts, err := r.getOptionsObject(options)
	if err != nil {
		return vm.Undefined, err
	}
	s, err := r.differenceSettings(since, opts, unitsTime, nil, temporal.UnitNanosecond, temporal.UnitSecond)
	if err != nil {
		return vm.Undefined, err
	}
	diff, err := temporal.DifferenceInstant(ns, other, s.increment, s.smallest, s.mode)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	d, err := temporal.InternalDuration{Time: diff}.ToDuration(s.largest)
	if err != nil {
		return vm.Undefined, r.err(err)
	}
	if since {
		d = d.Negated()
	}
	return r.createDuration(d)
}

func installInstant(r *temporalRealm) error {
	ctor, proto := r.newClass("Instant", 1, func(args []vm.Value, p protoRef) (vm.Value, error) {
		ns, err := r.toBigInt(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		if !validEpochNs(ns) {
			return vm.Undefined, r.rangeErr("epoch nanoseconds out of range")
		}
		return r.wrapNew(p, &tInstant{ns: ns})
	})

	r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
		ns, err := r.toTemporalInstant(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createInstant(new(big.Int).Set(ns))
	})
	r.static(ctor, "fromEpochMilliseconds", 1, func(args []vm.Value) (vm.Value, error) {
		ms, err := r.toNumber(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		if math.IsNaN(ms) || math.IsInf(ms, 0) || ms != math.Trunc(ms) {
			return vm.Undefined, r.rangeErr("epoch milliseconds must be an integer")
		}
		b, _ := new(big.Float).SetFloat64(ms).Int(nil)
		return r.createInstant(b.Mul(b, bigNsPerMs))
	})
	r.static(ctor, "fromEpochNanoseconds", 1, func(args []vm.Value) (vm.Value, error) {
		ns, err := r.toBigInt(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createInstant(ns)
	})
	r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
		a, err := r.toTemporalInstant(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		b, err := r.toTemporalInstant(argAt(args, 1))
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(a.Cmp(b))), nil
	})

	r.getter(proto, "epochMilliseconds", func(this vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NumberValue(float64(floorDivBig(s.ns, bigNsPerMs).Int64())), nil
	})
	r.getter(proto, "epochNanoseconds", func(this vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return vm.NewBigInt(new(big.Int).Set(s.ns)), nil
	})

	r.method(proto, "add", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.addDurationToInstant(1, s.ns, argAt(args, 0))
	})
	r.method(proto, "subtract", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.addDurationToInstant(-1, s.ns, argAt(args, 0))
	})
	r.method(proto, "until", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.differenceInstant(false, s.ns, argAt(args, 0), argAt(args, 1))
	})
	r.method(proto, "since", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.differenceInstant(true, s.ns, argAt(args, 0), argAt(args, 1))
	})

	r.method(proto, "round", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		roundTo := argAt(args, 0)
		if roundTo.IsUndefined() {
			return vm.Undefined, r.typeErr("round requires a roundTo argument")
		}
		var smallest temporal.Unit
		var increment int64 = 1
		mode := temporal.RoundHalfExpand
		if roundTo.Type() == vm.TypeString {
			u, ok := unitNames[roundTo.ToString()]
			if !ok || !u.IsTimeUnit() {
				return vm.Undefined, r.rangeErr(roundTo.ToString() + " is not a valid value for option smallestUnit")
			}
			smallest = u
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
			var ok bool
			if smallest, ok, err = r.validateUnit("smallestUnit", raw, present, unitsTime, false); err != nil || !ok {
				return vm.Undefined, err
			}
		}
		maximum := int64(86400*1e9) / smallest.NanosecondsPerUnit()
		if err := temporal.ValidateRoundingIncrement(increment, maximum, true); err != nil {
			return vm.Undefined, r.err(err)
		}
		return r.createInstant(roundTemporalInstant(s.ns, increment, smallest, mode))
	})

	r.method(proto, "equals", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		other, err := r.toTemporalInstant(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return vm.BooleanValue(s.ns.Cmp(other) == 0), nil
	})

	r.method(proto, "toString", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.instantToString(s.ns, argAt(args, 0))
	})
	r.method(proto, "toJSON", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.instantToString(s.ns, vm.Undefined)
	})
	r.method(proto, "toLocaleString", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		return r.instantToString(s.ns, vm.Undefined)
	})
	r.method(proto, "valueOf", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		return vm.Undefined, r.typeErr("use compare() or equals() to compare Temporal.Instant")
	})
	r.method(proto, "toZonedDateTimeISO", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		s, err := thisSlots[tInstant](r, "Instant")
		if err != nil {
			return vm.Undefined, err
		}
		tz, err := r.toTemporalTimeZone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createZonedDateTime(s.ns, tz)
	})
	return nil
}

// instantToString is Temporal.Instant.prototype.toString.
func (r *temporalRealm) instantToString(ns *big.Int, options vm.Value) (vm.Value, error) {
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
	tzItem, err := r.option(opts, "timeZone")
	if err != nil {
		return vm.Undefined, err
	}
	smallest, _, err := r.validateUnit("smallestUnit", raw, present, unitsTime, false)
	if err != nil {
		return vm.Undefined, err
	}
	if present && smallest == temporal.UnitHour {
		return vm.Undefined, r.rangeErr("smallestUnit hour is not allowed")
	}
	var tz temporal.TimeZone
	hasTZ := !tzItem.IsUndefined()
	if hasTZ {
		if tz, err = r.toTemporalTimeZone(tzItem); err != nil {
			return vm.Undefined, err
		}
	}
	precision, unit, increment := secondsStringPrecision(smallest, present, digits)
	rounded := roundTemporalInstant(ns, increment, unit, mode)
	if !validEpochNs(rounded) {
		return vm.Undefined, r.rangeErr("instant out of range")
	}
	offset := "Z"
	offsetNs := int64(0)
	if hasTZ {
		offsetNs = tz.OffsetNanosecondsAt(rounded)
		offset = temporal.FormatOffset(offsetNs, true)
	}
	local := temporal.EpochNsToDateTime(new(big.Int).Add(rounded, big.NewInt(offsetNs)))
	return vm.NewString(temporal.FormatDateTime(local, precision) + offset), nil
}

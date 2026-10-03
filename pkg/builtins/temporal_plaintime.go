package builtins

import (
	"math/big"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

var timeFieldNames = []string{fHour, fMinute, fSecond, fMillisecond, fMicrosecond, fNanosecond}

func init() {
	registerTemporalInstaller(func(r *temporalRealm) error {
		ctor, proto := r.newClass("PlainTime", 0, func(args []vm.Value, p protoRef) (vm.Value, error) {
			var f [6]int
			for i := range f {
				if v := argAt(args, i); !v.IsUndefined() {
					n, err := r.toIntegerWithTruncation(v)
					if err != nil {
						return vm.Undefined, err
					}
					f[i] = clampToInt(n)
				}
			}
			if !temporal.IsValidTime(f[0], f[1], f[2], f[3], f[4], f[5]) {
				return vm.Undefined, r.rangeErr("invalid time")
			}
			return r.wrapNew(p, &tPlainTime{temporal.Time{Hour: f[0], Minute: f[1], Second: f[2], Millisecond: f[3], Microsecond: f[4], Nanosecond: f[5]}})
		})

		r.static(ctor, "from", 1, func(args []vm.Value) (vm.Value, error) {
			t, err := r.toTemporalTime(argAt(args, 0), argAt(args, 1))
			if err != nil {
				return vm.Undefined, err
			}
			return r.createPlainTime(t)
		})
		r.static(ctor, "compare", 2, func(args []vm.Value) (vm.Value, error) {
			a, err := r.toTemporalTime(argAt(args, 0), vm.Undefined)
			if err != nil {
				return vm.Undefined, err
			}
			b, err := r.toTemporalTime(argAt(args, 1), vm.Undefined)
			if err != nil {
				return vm.Undefined, err
			}
			return vm.IntegerValue(int32(temporal.CompareTime(a, b))), nil
		})

		for name, get := range map[string]func(temporal.Time) int{
			fHour: func(t temporal.Time) int { return t.Hour }, fMinute: func(t temporal.Time) int { return t.Minute },
			fSecond: func(t temporal.Time) int { return t.Second }, fMillisecond: func(t temporal.Time) int { return t.Millisecond },
			fMicrosecond: func(t temporal.Time) int { return t.Microsecond }, fNanosecond: func(t temporal.Time) int { return t.Nanosecond },
		} {
			get := get
			r.getter(proto, name, func(this vm.Value) (vm.Value, error) {
				s, ok := slotsOf[tPlainTime](this)
				if !ok {
					return vm.Undefined, r.typeErr("Method called on incompatible receiver: not a Temporal.PlainTime")
				}
				return vm.IntegerValue(int32(get(s.time))), nil
			})
		}

		r.method(proto, "with", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainTime](r, "PlainTime")
			if err != nil {
				return vm.Undefined, err
			}
			item := argAt(args, 0)
			if !isObjectValue(item) {
				return vm.Undefined, r.typeErr("with() argument must be an object")
			}
			if err := r.rejectObjectWithCalendarOrTimeZone(item); err != nil {
				return vm.Undefined, err
			}
			f, err := r.prepareFields(item, timeFieldNames, nil, true)
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
			t, err := resolveTimeFields(f, s.time, overflow)
			if err != nil {
				return vm.Undefined, r.err(err)
			}
			return r.createPlainTime(t)
		})

		addSub := func(name string, sign int) {
			r.method(proto, name, 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
				s, err := thisSlots[tPlainTime](r, "PlainTime")
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
				// AddTime: only the time part of the duration counts.
				total := new(big.Int).Add(big.NewInt(s.time.Nanoseconds()), temporal.ToInternalDuration(d).Time)
				_, t := temporal.BalanceTime(total)
				return r.createPlainTime(t)
			})
		}
		addSub("add", 1)
		addSub("subtract", -1)

		diff := func(name string, since bool) {
			r.method(proto, name, 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
				s, err := thisSlots[tPlainTime](r, "PlainTime")
				if err != nil {
					return vm.Undefined, err
				}
				other, err := r.toTemporalTime(argAt(args, 0), vm.Undefined)
				if err != nil {
					return vm.Undefined, err
				}
				opts, err := r.getOptionsObject(argAt(args, 1))
				if err != nil {
					return vm.Undefined, err
				}
				set, err := r.differenceSettings(since, opts, unitsTime, nil, temporal.UnitNanosecond, temporal.UnitHour)
				if err != nil {
					return vm.Undefined, err
				}
				ns := big.NewInt(other.Nanoseconds() - s.time.Nanoseconds())
				ns, err = temporal.RoundTimeDuration(ns, set.increment, set.smallest, set.mode)
				if err != nil {
					return vm.Undefined, r.err(err)
				}
				d, err := temporal.InternalDuration{Time: ns}.ToDuration(set.largest)
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

		r.method(proto, "round", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainTime](r, "PlainTime")
			if err != nil {
				return vm.Undefined, err
			}
			roundTo := argAt(args, 0)
			if roundTo.IsUndefined() {
				return vm.Undefined, r.typeErr("round() requires an argument")
			}
			increment, mode := int64(1), temporal.RoundHalfExpand
			var unit temporal.Unit
			if roundTo.Type() == vm.TypeString {
				unit, _, err = r.validateUnit("smallestUnit", roundTo.ToString(), true, unitsTime, false)
				if err != nil {
					return vm.Undefined, err
				}
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
				if unit, _, err = r.unitOption(opts, "smallestUnit", unitsTime, unitRequired, false); err != nil {
					return vm.Undefined, err
				}
			}
			if max, ok := temporal.MaximumTemporalDurationRoundingIncrement(unit); ok {
				if err := temporal.ValidateRoundingIncrement(increment, max, false); err != nil {
					return vm.Undefined, r.err(err)
				}
			}
			_, t := temporal.RoundTime(s.time, increment, unit, mode)
			return r.createPlainTime(t)
		})

		r.method(proto, "equals", 1, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainTime](r, "PlainTime")
			if err != nil {
				return vm.Undefined, err
			}
			other, err := r.toTemporalTime(argAt(args, 0), vm.Undefined)
			if err != nil {
				return vm.Undefined, err
			}
			return vm.BooleanValue(s.time == other), nil
		})

		r.method(proto, "toString", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			s, err := thisSlots[tPlainTime](r, "PlainTime")
			if err != nil {
				return vm.Undefined, err
			}
			opts, err := r.getOptionsObject(argAt(args, 0))
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
			smallest, present, err := r.unitOption(opts, "smallestUnit", unitsTime, unitUnset, false)
			if err != nil {
				return vm.Undefined, err
			}
			if present && smallest == temporal.UnitHour {
				return vm.Undefined, r.rangeErr("smallestUnit hour is not allowed")
			}
			precision, unit, increment := secondsStringPrecision(smallest, present, digits)
			_, t := temporal.RoundTime(s.time, increment, unit, mode)
			return vm.NewString(temporal.FormatTime(t, precision)), nil
		})
		for _, name := range []string{"toLocaleString", "toJSON"} {
			r.method(proto, name, 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
				s, err := thisSlots[tPlainTime](r, "PlainTime")
				if err != nil {
					return vm.Undefined, err
				}
				return vm.NewString(temporal.FormatTime(s.time, -1)), nil
			})
		}
		r.method(proto, "valueOf", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
			return vm.Undefined, r.typeErr("Temporal.PlainTime cannot be converted to a primitive; use compare() or equals()")
		})
		return nil
	})
}

// resolveTimeFields overlays the present fields of f on base and regulates.
func resolveTimeFields(f *tFields, base temporal.Time, overflow temporal.Overflow) (temporal.Time, error) {
	pick := func(p *int, def int) int {
		if p != nil {
			return *p
		}
		return def
	}
	return temporal.RegulateTime(pick(f.Hour, base.Hour), pick(f.Minute, base.Minute), pick(f.Second, base.Second),
		pick(f.Millisecond, base.Millisecond), pick(f.Microsecond, base.Microsecond), pick(f.Nanosecond, base.Nanosecond), overflow)
}

// timeOfObject extracts the time of a PlainTime, PlainDateTime or
// ZonedDateTime.
func timeOfObject(item vm.Value) (temporal.Time, bool) {
	if s, ok := slotsOf[tPlainTime](item); ok {
		return s.time, true
	}
	if s, ok := slotsOf[tPlainDateTime](item); ok {
		return s.dt.Time, true
	}
	if s, ok := slotsOf[tZoned](item); ok {
		return s.tz.LocalDateTime(s.ns).Time, true
	}
	return temporal.Time{}, false
}

// toTemporalTime is ToTemporalTime.
func (r *temporalRealm) toTemporalTime(item, options vm.Value) (temporal.Time, error) {
	readOverflow := func() (temporal.Overflow, error) {
		opts, err := r.getOptionsObject(options)
		if err != nil {
			return 0, err
		}
		return r.overflowOption(opts)
	}
	if isObjectValue(item) {
		if t, ok := timeOfObject(item); ok {
			_, err := readOverflow()
			return t, err
		}
		f, err := r.prepareFields(item, timeFieldNames, nil, true)
		if err != nil {
			return temporal.Time{}, err
		}
		overflow, err := readOverflow()
		if err != nil {
			return temporal.Time{}, err
		}
		t, err := resolveTimeFields(f, temporal.Time{}, overflow)
		return t, r.err(err)
	}
	if item.Type() != vm.TypeString {
		return temporal.Time{}, r.typeErr("cannot convert value to a Temporal.PlainTime")
	}
	p, err := temporal.ParsePlainTimeString(item.ToString())
	if err != nil {
		return temporal.Time{}, r.err(err)
	}
	_, err = readOverflow()
	return p.Time, err
}

func (r *temporalRealm) createPlainTime(t temporal.Time) (vm.Value, error) {
	return r.wrap("PlainTime", &tPlainTime{t}), nil
}

package builtins

import (
	"math/big"
	"time"

	"github.com/nooga/paserati/pkg/temporal"
	"github.com/nooga/paserati/pkg/vm"
)

// Temporal.Now.

func init() { registerTemporalInstaller(installNow) }

// systemEpochNs is SystemUTCEpochNanoseconds, clamped to the Instant range.
func systemEpochNs() *big.Int {
	now := time.Now()
	ns := new(big.Int).Mul(big.NewInt(now.Unix()), big.NewInt(1e9))
	ns.Add(ns, big.NewInt(int64(now.Nanosecond())))
	if !temporal.InstantWithinLimits(ns) {
		return new(big.Int).Set(temporal.MaxEpochNanoseconds)
	}
	return ns
}

func installNow(r *temporalRealm) error {
	now := vm.NewObject(r.vm.ObjectPrototype).AsPlainObject()
	intlDefineToStringTag(r.vm, now, "Temporal.Now")
	r.ns.SetOwnNonEnumerable("Now", vm.NewValueFromPlainObject(now))

	// zone is the time zone argument of the *ISO functions: undefined means
	// the system time zone.
	zone := func(v vm.Value) (temporal.TimeZone, error) {
		if v.IsUndefined() {
			return temporal.SystemTimeZone(), nil
		}
		return r.toTemporalTimeZone(v)
	}

	r.method(now, "timeZoneId", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		return vm.NewString(temporal.SystemTimeZone().ID()), nil
	})
	r.method(now, "instant", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		return r.createInstant(systemEpochNs())
	})
	r.method(now, "plainDateTimeISO", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		tz, err := zone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDateTime(tz.LocalDateTime(systemEpochNs()))
	})
	r.method(now, "zonedDateTimeISO", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		tz, err := zone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createZonedDateTime(systemEpochNs(), tz)
	})
	r.method(now, "plainDateISO", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		tz, err := zone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainDate(tz.LocalDateTime(systemEpochNs()).Date)
	})
	r.method(now, "plainTimeISO", 0, func(this vm.Value, args []vm.Value) (vm.Value, error) {
		tz, err := zone(argAt(args, 0))
		if err != nil {
			return vm.Undefined, err
		}
		return r.createPlainTime(tz.LocalDateTime(systemEpochNs()).Time)
	})
	return nil
}

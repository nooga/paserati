package builtins

// Cross-type operations of the Temporal bindings. Each lives in the file of
// the type that owns it (so every file can be rewritten independently) and
// has exactly this signature; other types call them freely:
//
//   temporal_instant.go
//     func (r *temporalRealm) toTemporalInstant(item vm.Value) (*big.Int, error)
//     func (r *temporalRealm) createInstant(ns *big.Int) (vm.Value, error)
//   temporal_duration.go
//     func (r *temporalRealm) createDuration(d temporal.Duration) (vm.Value, error)
//     (toTemporalDuration is in temporal_common.go)
//   temporal_plaindate.go
//     func (r *temporalRealm) toTemporalDate(item, options vm.Value) (temporal.Date, error)
//     func (r *temporalRealm) createPlainDate(d temporal.Date) (vm.Value, error)
//   temporal_plaintime.go
//     func (r *temporalRealm) toTemporalTime(item, options vm.Value) (temporal.Time, error)
//     func (r *temporalRealm) createPlainTime(t temporal.Time) (vm.Value, error)
//   temporal_plaindatetime.go
//     func (r *temporalRealm) toTemporalDateTime(item, options vm.Value) (temporal.DateTime, error)
//     func (r *temporalRealm) createPlainDateTime(dt temporal.DateTime) (vm.Value, error)
//   temporal_plainyearmonth.go
//     func (r *temporalRealm) toTemporalYearMonth(item, options vm.Value) (temporal.Date, error)
//     func (r *temporalRealm) createPlainYearMonth(ref temporal.Date) (vm.Value, error)
//   temporal_plainmonthday.go
//     func (r *temporalRealm) toTemporalMonthDay(item, options vm.Value) (temporal.Date, error)
//     func (r *temporalRealm) createPlainMonthDay(ref temporal.Date) (vm.Value, error)
//   temporal_zoneddatetime.go
//     func (r *temporalRealm) toTemporalZonedDateTime(item, options vm.Value) (*tZoned, error)
//     func (r *temporalRealm) toTemporalTimeZone(item vm.Value) (temporal.TimeZone, error)
//     func (r *temporalRealm) createZonedDateTime(ns *big.Int, tz temporal.TimeZone) (vm.Value, error)
//
// The create* functions check the type's range and return a RangeError when
// the value is outside it. The toTemporal* functions implement the spec's
// ToTemporalX, including reading options in the spec's order and their
// overflow/disambiguation/offset handling for the "from" forms. A year-month
// carries a reference ISO date (day 1 unless the caller has a real day) and
// a month-day a reference year (1972 for strings and property bags).

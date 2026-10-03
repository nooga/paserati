package temporal

import (
	"fmt"
	"math/big"
)

// RangeError and TypeError tell the bindings which JS error to throw.
type RangeError struct{ Msg string }
type TypeError struct{ Msg string }

func (e *RangeError) Error() string { return e.Msg }
func (e *TypeError) Error() string  { return e.Msg }

func rangeErr(format string, a ...any) error { return &RangeError{fmt.Sprintf(format, a...)} }
func typeErr(format string, a ...any) error  { return &TypeError{fmt.Sprintf(format, a...)} }

// Unit is a Temporal unit, ordered from largest to smallest so that
// comparisons like `u < UnitDay` mean "a calendar unit larger than a day".
type Unit int

const (
	UnitAuto Unit = iota // "auto": not a real unit, used for defaults
	UnitYear
	UnitMonth
	UnitWeek
	UnitDay
	UnitHour
	UnitMinute
	UnitSecond
	UnitMillisecond
	UnitMicrosecond
	UnitNanosecond
)

func (u Unit) String() string {
	return [...]string{"auto", "year", "month", "week", "day", "hour", "minute", "second", "millisecond", "microsecond", "nanosecond"}[u]
}

// IsCalendarUnit reports whether u is year, month or week; IsDateUnit adds
// day; IsTimeUnit is hour and smaller.
func (u Unit) IsCalendarUnit() bool { return u >= UnitYear && u <= UnitWeek }
func (u Unit) IsDateUnit() bool     { return u >= UnitYear && u <= UnitDay }
func (u Unit) IsTimeUnit() bool     { return u >= UnitHour && u <= UnitNanosecond }

// NanosecondsPerUnit is the length of a time unit (hour and smaller, plus
// day as 24 hours) in nanoseconds.
func (u Unit) NanosecondsPerUnit() int64 {
	switch u {
	case UnitDay:
		return 86400 * 1e9
	case UnitHour:
		return 3600 * 1e9
	case UnitMinute:
		return 60 * 1e9
	case UnitSecond:
		return 1e9
	case UnitMillisecond:
		return 1e6
	case UnitMicrosecond:
		return 1e3
	case UnitNanosecond:
		return 1
	}
	return 0
}

// RoundingMode is one of the nine Temporal rounding modes.
type RoundingMode int

const (
	RoundCeil RoundingMode = iota
	RoundFloor
	RoundExpand
	RoundTrunc
	RoundHalfCeil
	RoundHalfFloor
	RoundHalfExpand // the default for most operations
	RoundHalfTrunc
	RoundHalfEven
)

// Overflow is the "overflow" option of date/time construction and with().
type Overflow int

const (
	OverflowConstrain Overflow = iota
	OverflowReject
)

// Disambiguation resolves a local time that is skipped or repeated by a
// time zone transition.
type Disambiguation int

const (
	DisambiguateCompatible Disambiguation = iota
	DisambiguateEarlier
	DisambiguateLater
	DisambiguateReject
)

// OffsetOption is the "offset" option of ZonedDateTime.from/with.
type OffsetOption int

const (
	OffsetPrefer OffsetOption = iota
	OffsetUse
	OffsetIgnore
	OffsetReject
)

// Date is an ISO 8601 calendar date (the spec's ISO Date Record).
type Date struct{ Year, Month, Day int }

// Time is a wall-clock time (the spec's Time Record).
type Time struct{ Hour, Minute, Second, Millisecond, Microsecond, Nanosecond int }

// DateTime is a date and a time (the spec's ISO Date-Time Record).
type DateTime struct {
	Date Date
	Time Time
}

// Duration is the ten fields of a Temporal.Duration. They hold integral
// values: the JS Number the user passed, or the result of arithmetic that
// was then validated (see IsValidDuration), so a float64 is exact for every
// valid duration's calendar fields and for time fields of ordinary size.
// Arithmetic on the time fields goes through big integers.
type Duration struct {
	Years, Months, Weeks, Days                                       float64
	Hours, Minutes, Seconds, Milliseconds, Microseconds, Nanoseconds float64
}

// MaxEpochNanoseconds bounds a Temporal.Instant: ±8.64e21 ns, ±1e8 days.
var MaxEpochNanoseconds = new(big.Int).Mul(big.NewInt(86400), new(big.Int).Mul(big.NewInt(1e8), big.NewInt(1e9)))

var bigNsPerDay = big.NewInt(86400 * 1e9)

// ---------------------------------------------------------------------------
// Parsing results (filled by parse.go)
// ---------------------------------------------------------------------------

// Parsed is the result of parsing any ISO 8601 / RFC 9557 date-time string.
// The Has* fields say which components the string had; the parse functions
// have already rejected strings that lack what their goal requires.
type Parsed struct {
	HasDate bool
	Date    Date

	HasTime bool
	Time    Time // a leap second (60) has been read as 59

	// UTC is true for a trailing "Z". HasOffset is true for a numeric UTC
	// offset (and false for "Z"). OffsetNs is its value in nanoseconds;
	// OffsetHasSubMinute says the string carried seconds or a fraction (an
	// offset of that precision is only a valid match for a zone offset with
	// the same precision).
	UTC                bool
	HasOffset          bool
	OffsetNs           int64
	OffsetHasSubMinute bool

	// TimeZone is the bracketed time zone annotation ("" if none), as written
	// (not yet canonicalized). TimeZoneCritical is set by a leading "!".
	TimeZone         string
	TimeZoneCritical bool

	// Calendar is the "u-ca=" annotation value ("" if none), as written and
	// not validated; CalendarCritical is set by a leading "!". A caller that
	// uses the calendar must reject an unsupported one; a critical one is an
	// error for every caller that goes on to use the calendar.
	Calendar         string
	CalendarCritical bool
}

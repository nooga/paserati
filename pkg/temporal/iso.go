package temporal

import "math/big"

// ISO 8601 calendar arithmetic (the spec's ISO8601 calendar abstract
// operations). Dates are proleptic Gregorian.

const (
	nsPerSecond = int64(1e9)
	nsPerDay    = int64(86400) * nsPerSecond
	minEpochDay = -100000001 // -271821-04-19, the PlainDate lower limit
	maxEpochDay = 100000000  // +275760-09-13
)

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

func IsLeapYear(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func DaysInMonth(y, m int) int {
	switch m {
	case 2:
		if IsLeapYear(y) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func DaysInYear(y int) int {
	if IsLeapYear(y) {
		return 366
	}
	return 365
}

func IsValidISODate(y, m, d int) bool {
	return m >= 1 && m <= 12 && d >= 1 && d <= DaysInMonth(y, m)
}

func IsValidTime(h, mi, s, ms, us, ns int) bool {
	return h >= 0 && h <= 23 && mi >= 0 && mi <= 59 && s >= 0 && s <= 59 &&
		ms >= 0 && ms <= 999 && us >= 0 && us <= 999 && ns >= 0 && ns <= 999
}

// daysFromCivil is Howard Hinnant's algorithm; m is 1..12.
func daysFromCivil(y, m, d int64) int64 {
	if m <= 2 {
		y--
	}
	era := floorDiv(y, 400)
	yoe := y - era*400
	mp := m + 9
	if m > 2 {
		mp = m - 3
	}
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// ISODateToEpochDays is the spec's ISODateToEpochDays: month and day may be
// out of range, and are balanced.
func ISODateToEpochDays(year, month, day int) int64 {
	y := int64(year) + floorDiv(int64(month)-1, 12)
	m := floorMod(int64(month)-1, 12) + 1
	return daysFromCivil(y, m, 1) + int64(day) - 1
}

func EpochDaysToISODate(days int64) Date {
	z := days + 719468
	era := floorDiv(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d := doy - (153*mp+2)/5 + 1
	m := mp + 3
	if mp >= 10 {
		m = mp - 9
	}
	if m <= 2 {
		y++
	}
	return Date{int(y), int(m), int(d)}
}

// EpochDays is the date's days since 1970-01-01.
func (d Date) EpochDays() int64 { return ISODateToEpochDays(d.Year, d.Month, d.Day) }

// DayOfWeek is 1 (Monday) to 7 (Sunday).
func (d Date) DayOfWeek() int { return int(floorMod(d.EpochDays()+3, 7)) + 1 }

func (d Date) DayOfYear() int {
	return int(d.EpochDays() - ISODateToEpochDays(d.Year, 1, 1) + 1)
}

func weeksInYear(y int) int {
	p := func(y int64) int64 { return floorMod(y+floorDiv(y, 4)-floorDiv(y, 100)+floorDiv(y, 400), 7) }
	if p(int64(y)) == 4 || p(int64(y)-1) == 3 {
		return 53
	}
	return 52
}

// WeekOfYear returns the ISO week number and the year that week belongs to.
func (d Date) WeekOfYear() (week, yearOfWeek int) {
	week = (d.DayOfYear() - d.DayOfWeek() + 10) / 7
	switch {
	case week < 1:
		return weeksInYear(d.Year - 1), d.Year - 1
	case week > weeksInYear(d.Year):
		return 1, d.Year + 1
	}
	return week, d.Year
}

func CompareISODate(a, b Date) int {
	switch {
	case a.Year != b.Year:
		return sgn(a.Year - b.Year)
	case a.Month != b.Month:
		return sgn(a.Month - b.Month)
	}
	return sgn(a.Day - b.Day)
}

func sgn(x int) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return 1
	}
	return 0
}

// Nanoseconds is the time's offset from midnight.
func (t Time) Nanoseconds() int64 {
	return ((int64(t.Hour)*60+int64(t.Minute))*60+int64(t.Second))*nsPerSecond +
		int64(t.Millisecond)*1e6 + int64(t.Microsecond)*1e3 + int64(t.Nanosecond)
}

// TimeFromNanoseconds converts an offset in [0, nsPerDay) from midnight.
func TimeFromNanoseconds(ns int64) Time {
	t := Time{}
	t.Nanosecond = int(ns % 1000)
	ns /= 1000
	t.Microsecond = int(ns % 1000)
	ns /= 1000
	t.Millisecond = int(ns % 1000)
	ns /= 1000
	t.Second = int(ns % 60)
	ns /= 60
	t.Minute = int(ns % 60)
	t.Hour = int(ns / 60)
	return t
}

func CompareTime(a, b Time) int {
	x, y := a.Nanoseconds(), b.Nanoseconds()
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// ISODateWithinLimits reports whether d is within the PlainDate range,
// -271821-04-19 through +275760-09-13.
func ISODateWithinLimits(d Date) bool {
	days := d.EpochDays()
	return days >= minEpochDay && days <= maxEpochDay
}

// GetUTCEpochNanoseconds is the epoch time of dt read as UTC.
func GetUTCEpochNanoseconds(dt DateTime) *big.Int {
	ns := new(big.Int).Mul(big.NewInt(dt.Date.EpochDays()), bigNsPerDay)
	return ns.Add(ns, big.NewInt(dt.Time.Nanoseconds()))
}

var bigDayWindow = new(big.Int).Add(MaxEpochNanoseconds, bigNsPerDay)

// ISODateTimeWithinLimits allows the Instant range extended by just under a
// day on each side, which keeps every valid ZonedDateTime's wall clock
// representable.
func ISODateTimeWithinLimits(dt DateTime) bool {
	if days := dt.Date.EpochDays(); days < minEpochDay-1 || days > maxEpochDay+1 {
		return false
	}
	// Both bounds are exclusive: -271821-04-19T00:00 and +275760-09-14T00:00
	// are outside.
	return GetUTCEpochNanoseconds(dt).CmpAbs(bigDayWindow) < 0
}

// EpochNsToDateTime is the UTC date-time of an epoch time (the spec's
// GetISODateTimeFor with a UTC zone).
func EpochNsToDateTime(ns *big.Int) DateTime {
	q, r := new(big.Int).DivMod(ns, bigNsPerDay, new(big.Int))
	return DateTime{EpochDaysToISODate(q.Int64()), TimeFromNanoseconds(r.Int64())}
}

func InstantWithinLimits(ns *big.Int) bool { return ns.CmpAbs(MaxEpochNanoseconds) <= 0 }

// RegulateISODate is the spec's RegulateISODate (month and day only; the
// caller range-checks the year).
func RegulateISODate(y, m, d int, overflow Overflow) (Date, error) {
	if overflow == OverflowReject {
		if !IsValidISODate(y, m, d) {
			return Date{}, rangeErr("invalid ISO date %d-%d-%d", y, m, d)
		}
		return Date{y, m, d}, nil
	}
	m = clampInt(m, 1, 12)
	d = clampInt(d, 1, DaysInMonth(y, m))
	return Date{y, m, d}, nil
}

// RegulateTime is the spec's RegulateTime.
func RegulateTime(h, mi, s, ms, us, ns int, overflow Overflow) (Time, error) {
	if overflow == OverflowReject {
		if !IsValidTime(h, mi, s, ms, us, ns) {
			return Time{}, rangeErr("invalid time")
		}
		return Time{h, mi, s, ms, us, ns}, nil
	}
	return Time{clampInt(h, 0, 23), clampInt(mi, 0, 59), clampInt(s, 0, 59),
		clampInt(ms, 0, 999), clampInt(us, 0, 999), clampInt(ns, 0, 999)}, nil
}

func clampInt(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// BalanceISODate normalizes an out-of-range month and day.
func BalanceISODate(y, m int, d int64) Date {
	days := ISODateToEpochDays(y, m, 1) + d - 1
	return EpochDaysToISODate(days)
}

// BalanceISOYearMonth normalizes a month outside 1..12 into the year.
func BalanceISOYearMonth(y int, m int64) (int, int) {
	yy := int64(y) + floorDiv(m-1, 12)
	return int(yy), int(floorMod(m-1, 12)) + 1
}

// BalanceTime balances a total of nanoseconds since midnight into whole days
// and a time of day.
func BalanceTime(totalNs *big.Int) (days int64, t Time) {
	q, r := new(big.Int).DivMod(totalNs, bigNsPerDay, new(big.Int))
	return q.Int64(), TimeFromNanoseconds(r.Int64())
}

// AddISODate is the spec's AddISODate for the ISO calendar.
func AddISODate(d Date, years, months, weeks, days int64, overflow Overflow) (Date, error) {
	if !ISODateWithinLimits(d) {
		return Date{}, rangeErr("date out of range")
	}
	y, m := BalanceISOYearMonth(int(int64(d.Year)+years), int64(d.Month)+months)
	reg, err := RegulateISODate(y, m, d.Day, overflow)
	if err != nil {
		return Date{}, err
	}
	res := BalanceISODate(reg.Year, reg.Month, int64(reg.Day)+7*weeks+days)
	if !ISODateWithinLimits(res) {
		return Date{}, rangeErr("date out of range")
	}
	return res, nil
}

// isoDateSurpasses is ISODateSurpasses: whether (y, m, d) is past other in
// direction sign. The day is constrained to its month first (Jan 31 plus a
// month is Feb 29, which does not surpass Feb 29).
func isoDateSurpasses(sign int, y, m, d int, other Date) bool {
	if m >= 1 && m <= 12 {
		if dim := DaysInMonth(y, m); d > dim {
			d = dim
		}
	}
	switch {
	case y != other.Year:
		return sign*(y-other.Year) > 0
	case m != other.Month:
		return sign*(m-other.Month) > 0
	case d != other.Day:
		return sign*(d-other.Day) > 0
	}
	return false
}

// DateDuration is the calendar part of a duration, signed per field.
type DateDuration struct{ Years, Months, Weeks, Days int64 }

func (d DateDuration) Sign() int {
	for _, v := range [...]int64{d.Years, d.Months, d.Weeks, d.Days} {
		if v < 0 {
			return -1
		}
		if v > 0 {
			return 1
		}
	}
	return 0
}

// largestNonSurpassing returns the largest n in 0..limit (stepping by sign)
// for which surpasses(n) is false, starting the search at guess. surpasses
// must be monotonic in n.
func largestNonSurpassing(sign int, guess int64, surpasses func(n int64) bool) int64 {
	n := guess
	for n != 0 && surpasses(n) {
		n -= int64(sign)
	}
	for !surpasses(n + int64(sign)) {
		n += int64(sign)
	}
	return n
}

// DifferenceISODate is CalendarDateUntil for the ISO calendar: the signed
// calendar duration from one to two, balanced up to largestUnit (year,
// month, week or day).
func DifferenceISODate(one, two Date, largestUnit Unit) DateDuration {
	sign := -CompareISODate(one, two)
	if sign == 0 {
		return DateDuration{}
	}
	var years, months, weeks, days int64
	if largestUnit == UnitYear {
		years = largestNonSurpassing(sign, int64(two.Year-one.Year), func(n int64) bool {
			return isoDateSurpasses(sign, int(int64(one.Year)+n), one.Month, one.Day, two)
		})
	}
	if largestUnit == UnitYear || largestUnit == UnitMonth {
		guess := (int64(two.Year)-int64(one.Year)-years)*12 + int64(two.Month-one.Month)
		months = largestNonSurpassing(sign, guess, func(n int64) bool {
			y, m := BalanceISOYearMonth(int(int64(one.Year)+years), int64(one.Month)+n)
			return isoDateSurpasses(sign, y, m, one.Day, two)
		})
	}
	y, m := BalanceISOYearMonth(int(int64(one.Year)+years), int64(one.Month)+months)
	constrained, _ := RegulateISODate(y, m, one.Day, OverflowConstrain)
	// The remaining difference is a whole number of days from the
	// constrained date to two, which never surpasses it.
	remaining := two.EpochDays() - constrained.EpochDays()
	if largestUnit == UnitWeek {
		weeks = remaining / 7
		days = remaining % 7
	} else {
		days = remaining
	}
	return DateDuration{years, months, weeks, days}
}

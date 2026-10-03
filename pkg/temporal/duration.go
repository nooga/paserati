package temporal

import (
	"math"
	"math/big"
)

// Durations, rounding and the "relative" difference algorithms
// (RoundRelativeDuration and its Nudge/Bubble helpers).

var (
	bigZero = big.NewInt(0)
	// maxTimeDuration is 2^53 * 10^9 - 1: the largest time duration, in ns.
	maxTimeDuration = new(big.Int).Sub(new(big.Int).Mul(new(big.Int).Lsh(big.NewInt(1), 53), big.NewInt(1e9)), big.NewInt(1))
)

// Zone is the part of a time zone the duration algorithms need.
type Zone interface {
	LocalDateTime(epochNs *big.Int) DateTime
	EpochNanosecondsFor(local DateTime, d Disambiguation) (*big.Int, error)
}

// RelativeTo is a Duration operation's relativeTo: a plain date (Zone nil,
// Time is midnight) or a zoned date-time (EpochNs is its instant, Date and
// Time its wall clock in Zone).
type RelativeTo struct {
	Date    Date
	Time    Time
	Zone    Zone
	EpochNs *big.Int
}

// InternalDuration is a duration as the spec's algorithms hold it: a date
// part and an exact time part in nanoseconds (days are NOT folded into it).
type InternalDuration struct {
	Date DateDuration
	Time *big.Int
}

func zeroInternal() InternalDuration { return InternalDuration{Time: new(big.Int)} }

func (i InternalDuration) sign() int {
	if s := i.Date.Sign(); s != 0 {
		return s
	}
	return i.Time.Sign()
}

func floatToBig(f float64) *big.Int {
	b, _ := new(big.Float).SetFloat64(f).Int(nil)
	return b
}

// DurationSign is -1, 0 or 1 by the sign of the first non-zero field.
func DurationSign(d Duration) int {
	for _, v := range [...]float64{d.Years, d.Months, d.Weeks, d.Days, d.Hours, d.Minutes, d.Seconds, d.Milliseconds, d.Microseconds, d.Nanoseconds} {
		if v < 0 {
			return -1
		}
		if v > 0 {
			return 1
		}
	}
	return 0
}

func (d Duration) fields() [10]float64 {
	return [10]float64{d.Years, d.Months, d.Weeks, d.Days, d.Hours, d.Minutes, d.Seconds, d.Milliseconds, d.Microseconds, d.Nanoseconds}
}

func durationFromFields(f [10]float64) Duration {
	return Duration{f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9]}
}

// timeNs is the exact value of the days..nanoseconds fields in nanoseconds
// (days counted as 24 hours).
func (d Duration) timeNsWithDays() *big.Int {
	t := new(big.Int).Mul(floatToBig(d.Days), bigNsPerDay)
	return t.Add(t, d.timeNs())
}

// timeNs is the exact value of the hours..nanoseconds fields in nanoseconds.
func (d Duration) timeNs() *big.Int {
	t := new(big.Int).Mul(floatToBig(d.Hours), big.NewInt(3600*1e9))
	t.Add(t, new(big.Int).Mul(floatToBig(d.Minutes), big.NewInt(60*1e9)))
	t.Add(t, new(big.Int).Mul(floatToBig(d.Seconds), big.NewInt(1e9)))
	t.Add(t, new(big.Int).Mul(floatToBig(d.Milliseconds), big.NewInt(1e6)))
	t.Add(t, new(big.Int).Mul(floatToBig(d.Microseconds), big.NewInt(1e3)))
	return t.Add(t, floatToBig(d.Nanoseconds))
}

// IsValidDuration is the spec's IsValidDuration.
func IsValidDuration(d Duration) bool {
	sign := 0
	for _, v := range d.fields() {
		if math.IsInf(v, 0) || math.IsNaN(v) || v != math.Trunc(v) {
			return false
		}
		s := 0
		if v < 0 {
			s = -1
		} else if v > 0 {
			s = 1
		}
		if s != 0 {
			if sign != 0 && s != sign {
				return false
			}
			sign = s
		}
	}
	limit := float64(1 << 32)
	if math.Abs(d.Years) >= limit || math.Abs(d.Months) >= limit || math.Abs(d.Weeks) >= limit {
		return false
	}
	return d.timeNsWithDays().CmpAbs(maxTimeDuration) <= 0
}

func (d Duration) Negated() Duration {
	f := d.fields()
	for i := range f {
		f[i] = -f[i]
		if f[i] == 0 {
			f[i] = 0 // normalize -0
		}
	}
	return durationFromFields(f)
}

func (d Duration) Abs() Duration {
	if DurationSign(d) < 0 {
		return d.Negated()
	}
	return d
}

// DefaultTemporalLargestUnit is the largest unit with a non-zero field.
func DefaultTemporalLargestUnit(d Duration) Unit {
	for i, v := range d.fields() {
		if v != 0 {
			return Unit(i + 1)
		}
	}
	return UnitNanosecond
}

func LargerOfTwoUnits(a, b Unit) Unit {
	if a < b {
		return a
	}
	return b
}

// ToInternalDuration is ToInternalDurationRecord.
func ToInternalDuration(d Duration) InternalDuration {
	return InternalDuration{
		Date: DateDuration{int64(d.Years), int64(d.Months), int64(d.Weeks), int64(d.Days)},
		Time: d.timeNs(),
	}
}

func checkTimeDuration(t *big.Int) error {
	if t.CmpAbs(maxTimeDuration) > 0 {
		return rangeErr("duration out of range")
	}
	return nil
}

// Add24HourDaysToTimeDuration is the spec's operation of the same name.
func Add24HourDaysToTimeDuration(t *big.Int, days int64) (*big.Int, error) {
	r := new(big.Int).Mul(big.NewInt(days), bigNsPerDay)
	r.Add(r, t)
	return r, checkTimeDuration(r)
}

// ToDuration is TemporalDurationFromInternal: the time part is balanced up
// to largestUnit (a calendar unit is treated as day), the date part is kept.
func (i InternalDuration) ToDuration(largestUnit Unit) (Duration, error) {
	sign := i.Time.Sign()
	t := new(big.Int).Abs(i.Time)
	var days, hours, minutes, seconds, ms, us, ns *big.Int
	zero := func() *big.Int { return new(big.Int) }
	days, hours, minutes, seconds, ms, us, ns = zero(), zero(), zero(), zero(), zero(), zero(), zero()
	divmod := func(unit int64, q **big.Int) {
		*q = new(big.Int)
		(*q).DivMod(t, big.NewInt(unit), t)
	}
	if largestUnit <= UnitDay {
		divmod(nsPerDay, &days)
		largestUnit = UnitHour
	}
	if largestUnit <= UnitHour {
		divmod(3600*1e9, &hours)
		largestUnit = UnitMinute
	}
	if largestUnit <= UnitMinute {
		divmod(60*1e9, &minutes)
		largestUnit = UnitSecond
	}
	if largestUnit <= UnitSecond {
		divmod(1e9, &seconds)
		largestUnit = UnitMillisecond
	}
	if largestUnit <= UnitMillisecond {
		divmod(1e6, &ms)
		largestUnit = UnitMicrosecond
	}
	if largestUnit <= UnitMicrosecond {
		divmod(1e3, &us)
	}
	ns = t
	f := func(b *big.Int) float64 {
		if b.Sign() == 0 {
			return 0 // never -0
		}
		v, _ := new(big.Float).SetInt(b).Float64()
		return v * float64(sign)
	}
	d := Duration{
		Years:        float64(i.Date.Years),
		Months:       float64(i.Date.Months),
		Weeks:        float64(i.Date.Weeks),
		Days:         float64(i.Date.Days) + f(days),
		Hours:        f(hours),
		Minutes:      f(minutes),
		Seconds:      f(seconds),
		Milliseconds: f(ms),
		Microseconds: f(us),
		Nanoseconds:  f(ns),
	}
	if !IsValidDuration(d) {
		return Duration{}, rangeErr("duration out of range")
	}
	return d, nil
}

// ---------------------------------------------------------------------------
// Rounding
// ---------------------------------------------------------------------------

type unsignedMode int

const (
	unsignedInfinity unsignedMode = iota
	unsignedZero
	unsignedHalfInfinity
	unsignedHalfZero
	unsignedHalfEven
)

func unsignedRoundingMode(m RoundingMode, negative bool) unsignedMode {
	switch m {
	case RoundCeil:
		if negative {
			return unsignedZero
		}
		return unsignedInfinity
	case RoundFloor:
		if negative {
			return unsignedInfinity
		}
		return unsignedZero
	case RoundExpand:
		return unsignedInfinity
	case RoundTrunc:
		return unsignedZero
	case RoundHalfCeil:
		if negative {
			return unsignedHalfZero
		}
		return unsignedHalfInfinity
	case RoundHalfFloor:
		if negative {
			return unsignedHalfInfinity
		}
		return unsignedHalfZero
	case RoundHalfExpand:
		return unsignedHalfInfinity
	case RoundHalfTrunc:
		return unsignedHalfZero
	}
	return unsignedHalfEven
}

// applyUnsignedRoundingMode rounds x, known to lie in [r1, r2] with r2 = r1+1
// (in units of the increment), per ApplyUnsignedRoundingMode.
func applyUnsignedRoundingMode(x *big.Rat, r1, r2 *big.Int, mode unsignedMode) *big.Int {
	if x.IsInt() && x.Num().Cmp(r1) == 0 {
		return r1
	}
	switch mode {
	case unsignedInfinity:
		return r2
	case unsignedZero:
		return r1
	}
	d1 := new(big.Rat).Sub(x, new(big.Rat).SetInt(r1))
	d2 := new(big.Rat).Sub(new(big.Rat).SetInt(r2), x)
	switch d1.Cmp(d2) {
	case -1:
		return r1
	case 1:
		return r2
	}
	switch mode {
	case unsignedHalfZero:
		return r1
	case unsignedHalfInfinity:
		return r2
	}
	if new(big.Int).And(r1, big.NewInt(1)).Sign() == 0 {
		return r1
	}
	return r2
}

// RoundNumberToIncrement rounds x to a multiple of increment (> 0).
func RoundNumberToIncrement(x, increment *big.Int, mode RoundingMode) *big.Int {
	q, r := new(big.Int).DivMod(x, increment, new(big.Int)) // floor division
	if r.Sign() == 0 {
		return new(big.Int).Set(x)
	}
	// x/increment lies in (q, q+1); work on the magnitude for the unsigned mode.
	negative := x.Sign() < 0
	var r1, r2 *big.Int
	frac := new(big.Rat).SetFrac(new(big.Int).Abs(x), increment)
	absFloor := new(big.Int).Quo(new(big.Int).Abs(x), increment)
	r1, r2 = absFloor, new(big.Int).Add(absFloor, big.NewInt(1))
	_ = q
	rounded := applyUnsignedRoundingMode(frac, r1, r2, unsignedRoundingMode(mode, negative))
	if negative {
		rounded = new(big.Int).Neg(rounded)
	}
	return rounded.Mul(rounded, increment)
}

// RoundNumberToIncrementAsIfPositive is RoundNumberToIncrementAsIfPositive:
// the rounding mode is applied as though x were positive, so that ceil and
// expand always go toward later times even for a negative epoch value. It is
// what rounding an Instant needs.
func RoundNumberToIncrementAsIfPositive(x, increment *big.Int, mode RoundingMode) *big.Int {
	q, r := new(big.Int).DivMod(x, increment, new(big.Int)) // floor division
	if r.Sign() == 0 {
		return new(big.Int).Set(x)
	}
	frac := new(big.Rat).SetFrac(x, increment)
	rounded := applyUnsignedRoundingMode(frac, q, new(big.Int).Add(q, big.NewInt(1)), unsignedRoundingMode(mode, false))
	return rounded.Mul(rounded, increment)
}

// RoundTimeDuration rounds a time duration (nanoseconds) to a multiple of
// increment units of unit (RoundTimeDurationToIncrement).
func RoundTimeDuration(t *big.Int, increment int64, unit Unit, mode RoundingMode) (*big.Int, error) {
	r := RoundNumberToIncrement(t, new(big.Int).Mul(big.NewInt(increment), big.NewInt(unit.NanosecondsPerUnit())), mode)
	return r, checkTimeDuration(r)
}

// TotalTimeDuration is the exact quantity of unit in t.
func TotalTimeDuration(t *big.Int, unit Unit) *big.Rat {
	return new(big.Rat).SetFrac(t, big.NewInt(unit.NanosecondsPerUnit()))
}

// RoundTime is RoundTime: it rounds a time of day and reports the whole days
// carried out of it.
func RoundTime(t Time, increment int64, unit Unit, mode RoundingMode) (days int64, r Time) {
	rounded := RoundNumberToIncrement(big.NewInt(t.Nanoseconds()),
		big.NewInt(increment*unit.NanosecondsPerUnit()), mode)
	return BalanceTime(rounded)
}

// RoundISODateTime rounds a date-time; the result must stay in range.
func RoundISODateTime(dt DateTime, increment int64, unit Unit, mode RoundingMode) (DateTime, error) {
	if !ISODateTimeWithinLimits(dt) {
		return DateTime{}, rangeErr("date-time out of range")
	}
	days, t := RoundTime(dt.Time, increment, unit, mode)
	d := BalanceISODate(dt.Date.Year, dt.Date.Month, int64(dt.Date.Day)+days)
	res := DateTime{d, t}
	if !ISODateTimeWithinLimits(res) {
		return DateTime{}, rangeErr("date-time out of range")
	}
	return res, nil
}

// MaximumTemporalDurationRoundingIncrement is the exclusive bound on a
// rounding increment for unit, or ok=false when there is none (days and
// larger).
func MaximumTemporalDurationRoundingIncrement(unit Unit) (max int64, ok bool) {
	switch unit {
	case UnitHour:
		return 24, true
	case UnitMinute, UnitSecond:
		return 60, true
	case UnitMillisecond, UnitMicrosecond, UnitNanosecond:
		return 1000, true
	}
	return 0, false
}

// ValidateRoundingIncrement is ValidateTemporalRoundingIncrement.
func ValidateRoundingIncrement(increment, dividend int64, inclusive bool) error {
	max := dividend
	if !inclusive {
		max = dividend - 1
	}
	if increment > max {
		return rangeErr("rounding increment out of range")
	}
	if dividend%increment != 0 {
		return rangeErr("rounding increment must divide evenly")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Epoch-time arithmetic
// ---------------------------------------------------------------------------

// AddInstant adds an exact time duration to an epoch time.
func AddInstant(epochNs, t *big.Int) (*big.Int, error) {
	r := new(big.Int).Add(epochNs, t)
	if !InstantWithinLimits(r) {
		return nil, rangeErr("instant out of range")
	}
	return r, nil
}

// DifferenceInstant is the exact difference ns2 - ns1 rounded as requested
// (DifferenceInstant): the result is a time duration in nanoseconds.
func DifferenceInstant(ns1, ns2 *big.Int, increment int64, unit Unit, mode RoundingMode) (*big.Int, error) {
	diff := new(big.Int).Sub(ns2, ns1)
	return RoundTimeDuration(diff, increment, unit, mode)
}

// AddZonedDateTime adds a duration to a zoned instant (AddZonedDateTime).
func AddZonedDateTime(epochNs *big.Int, zone Zone, dd DateDuration, timeNs *big.Int, overflow Overflow) (*big.Int, error) {
	if dd.Sign() == 0 {
		return AddInstant(epochNs, timeNs)
	}
	start := zone.LocalDateTime(epochNs)
	added, err := AddISODate(start.Date, dd.Years, dd.Months, dd.Weeks, dd.Days, overflow)
	if err != nil {
		return nil, err
	}
	mid, err := zone.EpochNanosecondsFor(DateTime{added, start.Time}, DisambiguateCompatible)
	if err != nil {
		return nil, err
	}
	return AddInstant(mid, timeNs)
}

func timeDifference(t1, t2 Time) int64 { return t2.Nanoseconds() - t1.Nanoseconds() }

// DifferenceISODateTime is DifferenceISODateTime for the ISO calendar.
func DifferenceISODateTime(dt1, dt2 DateTime, largestUnit Unit) (InternalDuration, error) {
	if !ISODateTimeWithinLimits(dt1) || !ISODateTimeWithinLimits(dt2) {
		return InternalDuration{}, rangeErr("date-time out of range")
	}
	timeNs := big.NewInt(timeDifference(dt1.Time, dt2.Time))
	timeSign := timeNs.Sign()
	dateSign := CompareISODate(dt2.Date, dt1.Date)
	adjusted := dt1.Date
	if timeSign == -dateSign {
		adjusted = BalanceISODate(adjusted.Year, adjusted.Month, int64(adjusted.Day)-int64(timeSign))
		timeNs.Add(timeNs, new(big.Int).Mul(big.NewInt(int64(-timeSign)), bigNsPerDay))
	}
	dateLargest := LargerOfTwoUnits(UnitDay, largestUnit)
	dd := DifferenceISODate(adjusted, dt2.Date, dateLargest)
	if largestUnit != dateLargest {
		var err error
		if timeNs, err = Add24HourDaysToTimeDuration(timeNs, dd.Days); err != nil {
			return InternalDuration{}, err
		}
		dd = DateDuration{dd.Years, dd.Months, dd.Weeks, 0}
	}
	return InternalDuration{dd, timeNs}, nil
}

// DifferenceZonedDateTime is DifferenceZonedDateTime for the ISO calendar.
func DifferenceZonedDateTime(ns1, ns2 *big.Int, zone Zone, largestUnit Unit) (InternalDuration, error) {
	cmp := ns2.Cmp(ns1)
	if cmp == 0 {
		return zeroInternal(), nil
	}
	start := zone.LocalDateTime(ns1)
	end := zone.LocalDateTime(ns2)
	if CompareISODate(start.Date, end.Date) == 0 {
		return InternalDuration{Time: new(big.Int).Sub(ns2, ns1)}, nil
	}
	sign := cmp
	maxDayCorrection := 1
	if sign == 1 {
		maxDayCorrection = 2
	}
	dayCorrection := 0
	timeNs := big.NewInt(timeDifference(start.Time, end.Time))
	if timeNs.Sign() == -sign {
		dayCorrection++
	}
	var intermediate Date
	success := false
	for dayCorrection <= maxDayCorrection && !success {
		intermediate = BalanceISODate(end.Date.Year, end.Date.Month, int64(end.Date.Day)-int64(dayCorrection*sign))
		mid, err := zone.EpochNanosecondsFor(DateTime{intermediate, start.Time}, DisambiguateCompatible)
		if err != nil {
			return InternalDuration{}, err
		}
		timeNs = new(big.Int).Sub(ns2, mid)
		if sign != -timeNs.Sign() {
			success = true
		}
		dayCorrection++
	}
	dateLargest := LargerOfTwoUnits(UnitDay, largestUnit)
	dd := DifferenceISODate(start.Date, intermediate, dateLargest)
	return InternalDuration{dd, timeNs}, nil
}

// ---------------------------------------------------------------------------
// Relative rounding
// ---------------------------------------------------------------------------

type nudgeResult struct {
	duration  InternalDuration
	nudgedNs  *big.Int
	didExpand bool
	total     *big.Rat
}

// originEpochNs is the epoch time of a wall-clock date-time in the origin
// zone, or its UTC reading when there is none.
func originEpochNs(dt DateTime, zone Zone) (*big.Int, error) {
	if zone == nil {
		return GetUTCEpochNanoseconds(dt), nil
	}
	return zone.EpochNanosecondsFor(dt, DisambiguateCompatible)
}

func ratFromInt(i int64) *big.Rat { return new(big.Rat).SetInt64(i) }

func absBig(x int64) *big.Int { return new(big.Int).Abs(big.NewInt(x)) }

// nudgeToCalendarUnit is NudgeToCalendarUnit.
func nudgeToCalendarUnit(sign int, dur InternalDuration, destNs *big.Int, origin DateTime, zone Zone, increment int64, unit Unit, mode RoundingMode) (nudgeResult, error) {
	var r1, r2 int64
	var startDD, endDD DateDuration
	inc := increment * int64(sign)
	trunc := func(v int64) int64 {
		return RoundNumberToIncrement(big.NewInt(v), big.NewInt(increment), RoundTrunc).Int64()
	}
	switch unit {
	case UnitYear:
		r1 = trunc(dur.Date.Years)
		r2 = r1 + inc
		startDD, endDD = DateDuration{r1, 0, 0, 0}, DateDuration{r2, 0, 0, 0}
	case UnitMonth:
		r1 = trunc(dur.Date.Months)
		r2 = r1 + inc
		startDD, endDD = DateDuration{dur.Date.Years, r1, 0, 0}, DateDuration{dur.Date.Years, r2, 0, 0}
	case UnitWeek:
		ym, err := AddISODate(origin.Date, dur.Date.Years, dur.Date.Months, 0, 0, OverflowConstrain)
		if err != nil {
			return nudgeResult{}, err
		}
		weeksEnd := BalanceISODate(ym.Year, ym.Month, int64(ym.Day)+dur.Date.Days)
		until := DifferenceISODate(ym, weeksEnd, UnitWeek)
		r1 = trunc(dur.Date.Weeks + until.Weeks)
		r2 = r1 + inc
		startDD = DateDuration{dur.Date.Years, dur.Date.Months, r1, 0}
		endDD = DateDuration{dur.Date.Years, dur.Date.Months, r2, 0}
	default: // day
		r1 = trunc(dur.Date.Days)
		r2 = r1 + inc
		startDD = DateDuration{dur.Date.Years, dur.Date.Months, dur.Date.Weeks, r1}
		endDD = DateDuration{dur.Date.Years, dur.Date.Months, dur.Date.Weeks, r2}
	}
	start, err := AddISODate(origin.Date, startDD.Years, startDD.Months, startDD.Weeks, startDD.Days, OverflowConstrain)
	if err != nil {
		return nudgeResult{}, err
	}
	end, err := AddISODate(origin.Date, endDD.Years, endDD.Months, endDD.Weeks, endDD.Days, OverflowConstrain)
	if err != nil {
		return nudgeResult{}, err
	}
	startNs, err := originEpochNs(DateTime{start, origin.Time}, zone)
	if err != nil {
		return nudgeResult{}, err
	}
	endNs, err := originEpochNs(DateTime{end, origin.Time}, zone)
	if err != nil {
		return nudgeResult{}, err
	}
	span := new(big.Int).Sub(endNs, startNs)
	if span.Sign() == 0 {
		return nudgeResult{}, rangeErr("cannot round: zero-length calendar unit")
	}
	// progress = (dest - start) / (end - start); total = r1 + progress*inc*sign... in
	// magnitude terms: total = r1 + progress * increment * sign.
	progress := new(big.Rat).SetFrac(new(big.Int).Sub(destNs, startNs), span)
	total := new(big.Rat).Mul(progress, ratFromInt(inc))
	total.Add(total, ratFromInt(r1))
	absTotal := new(big.Rat).Abs(total)
	rounded := applyUnsignedRoundingMode(absTotal, absBig(r1), absBig(r2), unsignedRoundingMode(mode, sign < 0))
	res := nudgeResult{total: total}
	if rounded.Cmp(absBig(r2)) == 0 {
		res.didExpand = true
		res.duration = InternalDuration{endDD, new(big.Int)}
		res.nudgedNs = endNs
	} else {
		res.duration = InternalDuration{startDD, new(big.Int)}
		res.nudgedNs = startNs
	}
	return res, nil
}

// nudgeToZonedTime is NudgeToZonedTime.
func nudgeToZonedTime(sign int, dur InternalDuration, origin DateTime, zone Zone, increment int64, unit Unit, mode RoundingMode) (nudgeResult, error) {
	start, err := AddISODate(origin.Date, dur.Date.Years, dur.Date.Months, dur.Date.Weeks, dur.Date.Days, OverflowConstrain)
	if err != nil {
		return nudgeResult{}, err
	}
	endDate := BalanceISODate(start.Year, start.Month, int64(start.Day)+int64(sign))
	startNs, err := zone.EpochNanosecondsFor(DateTime{start, origin.Time}, DisambiguateCompatible)
	if err != nil {
		return nudgeResult{}, err
	}
	endNs, err := zone.EpochNanosecondsFor(DateTime{endDate, origin.Time}, DisambiguateCompatible)
	if err != nil {
		return nudgeResult{}, err
	}
	daySpan := new(big.Int).Sub(endNs, startNs)
	rounded, err := RoundTimeDuration(dur.Time, increment, unit, mode)
	if err != nil {
		return nudgeResult{}, err
	}
	beyond := new(big.Int).Sub(rounded, daySpan)
	var dayDelta int64
	var nudged *big.Int
	didRoundBeyond := false
	if beyond.Sign() != -sign {
		didRoundBeyond = true
		dayDelta = int64(sign)
		if rounded, err = RoundTimeDuration(beyond, increment, unit, mode); err != nil {
			return nudgeResult{}, err
		}
		nudged = new(big.Int).Add(rounded, endNs)
	} else {
		nudged = new(big.Int).Add(rounded, startNs)
	}
	dd := dur.Date
	dd.Days += dayDelta
	return nudgeResult{duration: InternalDuration{dd, rounded}, nudgedNs: nudged, didExpand: didRoundBeyond}, nil
}

// nudgeToDayOrTime is NudgeToDayOrTime.
func nudgeToDayOrTime(dur InternalDuration, destNs *big.Int, largestUnit Unit, increment int64, unit Unit, mode RoundingMode) (nudgeResult, error) {
	timeNs, err := Add24HourDaysToTimeDuration(dur.Time, dur.Date.Days)
	if err != nil {
		return nudgeResult{}, err
	}
	rounded, err := RoundTimeDuration(timeNs, increment, unit, mode)
	if err != nil {
		return nudgeResult{}, err
	}
	beyond := new(big.Int).Sub(rounded, timeNs)
	wholeDays := new(big.Int).Quo(timeNs, bigNsPerDay)
	roundedWholeDays := new(big.Int).Quo(rounded, bigNsPerDay)
	dayDelta := new(big.Int).Sub(roundedWholeDays, wholeDays)
	didExpand := dayDelta.Sign() == timeNs.Sign()
	nudged := new(big.Int).Add(beyond, destNs)
	days := int64(0)
	remainder := rounded
	if LargerOfTwoUnits(largestUnit, UnitDay) == largestUnit {
		days = roundedWholeDays.Int64()
		remainder = new(big.Int).Sub(rounded, new(big.Int).Mul(roundedWholeDays, bigNsPerDay))
	}
	dd := dur.Date
	dd.Days = days
	return nudgeResult{duration: InternalDuration{dd, remainder}, nudgedNs: nudged, didExpand: didExpand}, nil
}

// bubbleRelativeDuration is BubbleRelativeDuration: after rounding up into a
// calendar unit, carry into the next larger units where the result reaches
// them.
func bubbleRelativeDuration(sign int, dur InternalDuration, nudgedNs *big.Int, origin DateTime, zone Zone, largestUnit, smallestUnit Unit) (InternalDuration, error) {
	if smallestUnit == UnitYear {
		return dur, nil
	}
	for unit := smallestUnit - 1; unit >= largestUnit && unit >= UnitYear; unit-- {
		if unit == UnitWeek && largestUnit != UnitWeek {
			continue
		}
		var end DateDuration
		s := int64(sign)
		switch unit {
		case UnitYear:
			end = DateDuration{dur.Date.Years + s, 0, 0, 0}
		case UnitMonth:
			end = DateDuration{dur.Date.Years, dur.Date.Months + s, 0, 0}
		case UnitWeek:
			end = DateDuration{dur.Date.Years, dur.Date.Months, dur.Date.Weeks + s, 0}
		default:
			end = DateDuration{dur.Date.Years, dur.Date.Months, dur.Date.Weeks, dur.Date.Days + s}
		}
		endDate, err := AddISODate(origin.Date, end.Years, end.Months, end.Weeks, end.Days, OverflowConstrain)
		if err != nil {
			return InternalDuration{}, err
		}
		endNs, err := originEpochNs(DateTime{endDate, origin.Time}, zone)
		if err != nil {
			return InternalDuration{}, err
		}
		if nudgedNs.Cmp(endNs) == -sign {
			break
		}
		dur = InternalDuration{end, new(big.Int)}
	}
	return dur, nil
}

// RoundRelativeDuration is RoundRelativeDuration. zone is nil for a plain
// (floating) origin.
func RoundRelativeDuration(dur InternalDuration, destNs *big.Int, origin DateTime, zone Zone, largestUnit Unit, increment int64, smallestUnit Unit, mode RoundingMode) (InternalDuration, error) {
	irregular := smallestUnit.IsCalendarUnit() || (zone != nil && smallestUnit == UnitDay)
	sign := 1
	if dur.sign() < 0 {
		sign = -1
	}
	var nudge nudgeResult
	var err error
	switch {
	case irregular:
		nudge, err = nudgeToCalendarUnit(sign, dur, destNs, origin, zone, increment, smallestUnit, mode)
	case zone != nil:
		nudge, err = nudgeToZonedTime(sign, dur, origin, zone, increment, smallestUnit, mode)
	default:
		nudge, err = nudgeToDayOrTime(dur, destNs, largestUnit, increment, smallestUnit, mode)
	}
	if err != nil {
		return InternalDuration{}, err
	}
	dur = nudge.duration
	if nudge.didExpand && smallestUnit != UnitWeek {
		startUnit := LargerOfTwoUnits(smallestUnit, UnitDay)
		return bubbleRelativeDuration(sign, dur, nudge.nudgedNs, origin, zone, largestUnit, startUnit)
	}
	return dur, nil
}

// TotalRelativeDuration is TotalRelativeDuration: the exact quantity of unit
// in the duration relative to an origin.
func TotalRelativeDuration(dur InternalDuration, destNs *big.Int, origin DateTime, zone Zone, unit Unit) (*big.Rat, error) {
	if unit.IsCalendarUnit() || (zone != nil && unit == UnitDay) {
		sign := 1
		if dur.sign() < 0 {
			sign = -1
		}
		n, err := nudgeToCalendarUnit(sign, dur, destNs, origin, zone, 1, unit, RoundTrunc)
		if err != nil {
			return nil, err
		}
		return n.total, nil
	}
	t, err := Add24HourDaysToTimeDuration(dur.Time, dur.Date.Days)
	if err != nil {
		return nil, err
	}
	return TotalTimeDuration(t, unit), nil
}

// DifferencePlainDateTimeWithRounding is the spec's operation of that name.
func DifferencePlainDateTimeWithRounding(dt1, dt2 DateTime, largestUnit Unit, increment int64, smallestUnit Unit, mode RoundingMode) (InternalDuration, error) {
	if GetUTCEpochNanoseconds(dt1).Cmp(GetUTCEpochNanoseconds(dt2)) == 0 {
		return zeroInternal(), nil
	}
	diff, err := DifferenceISODateTime(dt1, dt2, largestUnit)
	if err != nil {
		return InternalDuration{}, err
	}
	if smallestUnit == UnitNanosecond && increment == 1 {
		return diff, nil
	}
	return RoundRelativeDuration(diff, GetUTCEpochNanoseconds(dt2), dt1, nil, largestUnit, increment, smallestUnit, mode)
}

// DifferenceZonedDateTimeWithRounding is the spec's operation of that name.
func DifferenceZonedDateTimeWithRounding(ns1, ns2 *big.Int, zone Zone, largestUnit Unit, increment int64, smallestUnit Unit, mode RoundingMode) (InternalDuration, error) {
	if largestUnit.IsTimeUnit() {
		t, err := DifferenceInstant(ns1, ns2, increment, smallestUnit, mode)
		return InternalDuration{Time: t}, err
	}
	diff, err := DifferenceZonedDateTime(ns1, ns2, zone, largestUnit)
	if err != nil {
		return InternalDuration{}, err
	}
	if smallestUnit == UnitNanosecond && increment == 1 {
		return diff, nil
	}
	return RoundRelativeDuration(diff, ns2, zone.LocalDateTime(ns1), zone, largestUnit, increment, smallestUnit, mode)
}

// ---------------------------------------------------------------------------
// Duration operations
// ---------------------------------------------------------------------------

// RoundDuration is Duration.prototype.round after option reading. rel may
// be nil; calendar units then need it.
func RoundDuration(d Duration, largestUnit Unit, increment int64, smallestUnit Unit, mode RoundingMode, rel *RelativeTo) (Duration, error) {
	// A blank duration stays blank: return before the relativeTo is converted
	// to a date-time (which could be out of range).
	if DurationSign(d) == 0 && rel != nil && !(rel.Zone != nil && (largestUnit.IsDateUnit() || smallestUnit.IsDateUnit())) {
		return Duration{}, nil
	}
	hasCalendar := d.Years != 0 || d.Months != 0 || d.Weeks != 0
	needsRel := largestUnit.IsCalendarUnit() || smallestUnit.IsCalendarUnit() || hasCalendar
	var internal InternalDuration
	switch {
	case rel != nil && rel.Zone != nil:
		target, err := AddZonedDateTime(rel.EpochNs, rel.Zone, ToInternalDuration(d).Date, ToInternalDuration(d).Time, OverflowConstrain)
		if err != nil {
			return Duration{}, err
		}
		if internal, err = DifferenceZonedDateTimeWithRounding(rel.EpochNs, target, rel.Zone, largestUnit, increment, smallestUnit, mode); err != nil {
			return Duration{}, err
		}
	case rel != nil:
		in := ToInternalDuration(d)
		days, tm := BalanceTime(in.Time)
		dd := in.Date
		dd.Days += days
		target, err := AddISODate(rel.Date, dd.Years, dd.Months, dd.Weeks, dd.Days, OverflowConstrain)
		if err != nil {
			return Duration{}, err
		}
		origin := DateTime{rel.Date, Time{}}
		if internal, err = DifferencePlainDateTimeWithRounding(origin, DateTime{target, tm}, largestUnit, increment, smallestUnit, mode); err != nil {
			return Duration{}, err
		}
	default:
		if needsRel {
			return Duration{}, rangeErr("a relativeTo is required to round a duration with calendar units")
		}
		t, err := RoundTimeDuration(d.timeNsWithDays(), increment, smallestUnit, mode)
		if err != nil {
			return Duration{}, err
		}
		internal = InternalDuration{Time: t}
	}
	return internal.ToDuration(largestUnit)
}

// TotalDuration is Duration.prototype.total after option reading.
func TotalDuration(d Duration, unit Unit, rel *RelativeTo) (*big.Rat, error) {
	if DurationSign(d) == 0 && rel != nil && !(rel.Zone != nil && unit.IsDateUnit()) {
		return new(big.Rat), nil
	}
	hasCalendar := d.Years != 0 || d.Months != 0 || d.Weeks != 0
	in := ToInternalDuration(d)
	switch {
	case rel != nil && rel.Zone != nil:
		target, err := AddZonedDateTime(rel.EpochNs, rel.Zone, in.Date, in.Time, OverflowConstrain)
		if err != nil {
			return nil, err
		}
		diff, err := DifferenceZonedDateTime(rel.EpochNs, target, rel.Zone, UnitDay)
		if err != nil {
			return nil, err
		}
		if unit.IsTimeUnit() || unit == UnitDay {
			diff, err = DifferenceZonedDateTime(rel.EpochNs, target, rel.Zone, LargerOfTwoUnits(unit, UnitDay))
			if err != nil {
				return nil, err
			}
		} else if diff, err = DifferenceZonedDateTime(rel.EpochNs, target, rel.Zone, unit); err != nil {
			return nil, err
		}
		return TotalRelativeDuration(diff, target, rel.Zone.LocalDateTime(rel.EpochNs), rel.Zone, unit)
	case rel != nil:
		days, tm := BalanceTime(in.Time)
		dd := in.Date
		dd.Days += days
		target, err := AddISODate(rel.Date, dd.Years, dd.Months, dd.Weeks, dd.Days, OverflowConstrain)
		if err != nil {
			return nil, err
		}
		origin := DateTime{rel.Date, Time{}}
		end := DateTime{target, tm}
		diff, err := DifferenceISODateTime(origin, end, LargerOfTwoUnits(unit, UnitDay))
		if err != nil {
			return nil, err
		}
		return TotalRelativeDuration(diff, GetUTCEpochNanoseconds(end), origin, nil, unit)
	}
	if hasCalendar || unit.IsCalendarUnit() {
		return nil, rangeErr("a relativeTo is required to total a duration with calendar units")
	}
	return TotalTimeDuration(d.timeNsWithDays(), unit), nil
}

// AddDurations is AddDurations: sign is +1 for add and -1 for subtract.
func AddDurations(d1, d2 Duration, sign int, rel *RelativeTo) (Duration, error) {
	if sign < 0 {
		d2 = d2.Negated()
	}
	largest := LargerOfTwoUnits(DefaultTemporalLargestUnit(d1), DefaultTemporalLargestUnit(d2))
	in1, in2 := ToInternalDuration(d1), ToInternalDuration(d2)
	if rel == nil {
		if largest.IsCalendarUnit() {
			return Duration{}, rangeErr("a relativeTo is required to add durations with calendar units")
		}
		t := new(big.Int).Add(d1.timeNsWithDays(), d2.timeNsWithDays())
		if err := checkTimeDuration(t); err != nil {
			return Duration{}, err
		}
		return InternalDuration{Time: t}.ToDuration(largest)
	}
	timeSum := new(big.Int).Add(in1.Time, in2.Time)
	if err := checkTimeDuration(timeSum); err != nil {
		return Duration{}, err
	}
	if rel.Zone == nil {
		mid, err := AddISODate(rel.Date, in1.Date.Years, in1.Date.Months, in1.Date.Weeks, in1.Date.Days, OverflowConstrain)
		if err != nil {
			return Duration{}, err
		}
		end, err := AddISODate(mid, in2.Date.Years, in2.Date.Months, in2.Date.Weeks, in2.Date.Days, OverflowConstrain)
		if err != nil {
			return Duration{}, err
		}
		dd := DifferenceISODate(rel.Date, end, LargerOfTwoUnits(UnitDay, largest))
		return InternalDuration{dd, timeSum}.ToDuration(largest)
	}
	mid, err := AddZonedDateTime(rel.EpochNs, rel.Zone, in1.Date, in1.Time, OverflowConstrain)
	if err != nil {
		return Duration{}, err
	}
	end, err := AddZonedDateTime(mid, rel.Zone, in2.Date, in2.Time, OverflowConstrain)
	if err != nil {
		return Duration{}, err
	}
	diff, err := DifferenceZonedDateTimeWithRounding(rel.EpochNs, end, rel.Zone, largest, 1, UnitNanosecond, RoundTrunc)
	if err != nil {
		return Duration{}, err
	}
	return diff.ToDuration(largest)
}

// CompareDurations is Duration.compare: -1, 0 or 1.
func CompareDurations(d1, d2 Duration, rel *RelativeTo) (int, error) {
	if d1 == d2 {
		return 0, nil
	}
	in1, in2 := ToInternalDuration(d1), ToInternalDuration(d2)
	calendar := d1.Years != 0 || d1.Months != 0 || d1.Weeks != 0 || d2.Years != 0 || d2.Months != 0 || d2.Weeks != 0
	if rel != nil && rel.Zone != nil && (calendar || d1.Days != 0 || d2.Days != 0) {
		n1, err := AddZonedDateTime(rel.EpochNs, rel.Zone, in1.Date, in1.Time, OverflowConstrain)
		if err != nil {
			return 0, err
		}
		n2, err := AddZonedDateTime(rel.EpochNs, rel.Zone, in2.Date, in2.Time, OverflowConstrain)
		if err != nil {
			return 0, err
		}
		return n1.Cmp(n2), nil
	}
	if calendar {
		if rel == nil {
			return 0, rangeErr("a relativeTo is required to compare durations with calendar units")
		}
		// Fold each time part into whole days first, then add to the date.
		target := func(in InternalDuration) (Date, Time, error) {
			days, tm := BalanceTime(in.Time)
			date, err := AddISODate(rel.Date, in.Date.Years, in.Date.Months, in.Date.Weeks, in.Date.Days+days, OverflowConstrain)
			return date, tm, err
		}
		a, ta, err := target(in1)
		if err != nil {
			return 0, err
		}
		b, tb, err := target(in2)
		if err != nil {
			return 0, err
		}
		if c := CompareISODate(a, b); c != 0 {
			return c, nil
		}
		return CompareTime(ta, tb), nil
	}
	return d1.timeNsWithDays().Cmp(d2.timeNsWithDays()), nil
}

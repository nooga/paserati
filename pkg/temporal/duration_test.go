package temporal

import (
	"math/big"
	"testing"
)

// utcZone is a trivial Zone for tests that don't need transitions.
type utcZone struct{}

func (utcZone) LocalDateTime(ns *big.Int) DateTime { return EpochNsToDateTime(ns) }
func (utcZone) EpochNanosecondsFor(dt DateTime, _ Disambiguation) (*big.Int, error) {
	return GetUTCEpochNanoseconds(dt), nil
}

func TestRoundNumberToIncrement(t *testing.T) {
	modes := []RoundingMode{RoundCeil, RoundFloor, RoundExpand, RoundTrunc, RoundHalfCeil, RoundHalfFloor, RoundHalfExpand, RoundHalfTrunc, RoundHalfEven}
	cases := []struct {
		x    int64
		want [9]int64
	}{
		{25, [9]int64{30, 20, 30, 20, 30, 20, 30, 20, 20}},
		{-25, [9]int64{-20, -30, -30, -20, -20, -30, -30, -20, -20}},
		{35, [9]int64{40, 30, 40, 30, 40, 30, 40, 30, 40}},
		{-35, [9]int64{-30, -40, -40, -30, -30, -40, -40, -30, -40}},
		{22, [9]int64{30, 20, 30, 20, 20, 20, 20, 20, 20}},
		{28, [9]int64{30, 20, 30, 20, 30, 30, 30, 30, 30}},
		{30, [9]int64{30, 30, 30, 30, 30, 30, 30, 30, 30}},
		{0, [9]int64{0, 0, 0, 0, 0, 0, 0, 0, 0}},
	}
	for _, c := range cases {
		for i, m := range modes {
			if got := RoundNumberToIncrement(big.NewInt(c.x), big.NewInt(10), m).Int64(); got != c.want[i] {
				t.Errorf("round(%d, 10, mode %d) = %d, want %d", c.x, m, got, c.want[i])
			}
		}
	}
}

func TestIsValidDuration(t *testing.T) {
	valid := []Duration{{}, {Years: 1, Months: 2}, {Days: -3, Hours: -4}, {Hours: 9007199254740991 / 3600}}
	invalid := []Duration{
		{Years: 1, Months: -1},
		{Years: 4294967296},
		{Weeks: -4294967296},
		{Seconds: 9007199254740992},
		{Days: 104249991375, Hours: 1}, // days*86400 exceeds 2^53 seconds
		{Hours: 1.5},
	}
	for _, d := range valid {
		if !IsValidDuration(d) {
			t.Errorf("%+v should be valid", d)
		}
	}
	for _, d := range invalid {
		if IsValidDuration(d) {
			t.Errorf("%+v should be invalid", d)
		}
	}
}

func TestDefaultLargestUnitAndSign(t *testing.T) {
	if u := DefaultTemporalLargestUnit(Duration{Hours: 1, Seconds: 5}); u != UnitHour {
		t.Errorf("largest = %v", u)
	}
	if u := DefaultTemporalLargestUnit(Duration{}); u != UnitNanosecond {
		t.Errorf("zero largest = %v", u)
	}
	if DurationSign(Duration{Days: -1}) != -1 || DurationSign(Duration{}) != 0 {
		t.Error("sign")
	}
}

func TestRoundAndTotalWithoutRelativeTo(t *testing.T) {
	round := func(d Duration, largest, smallest Unit, inc int64, mode RoundingMode) Duration {
		t.Helper()
		got, err := RoundDuration(d, largest, inc, smallest, mode, nil)
		if err != nil {
			t.Fatalf("RoundDuration(%+v): %v", d, err)
		}
		return got
	}
	if got := round(Duration{Hours: 130, Minutes: 20}, UnitDay, UnitNanosecond, 1, RoundHalfExpand); got != (Duration{Days: 5, Hours: 10, Minutes: 20}) {
		t.Errorf("130h20m by day = %+v", got)
	}
	if got := round(Duration{Hours: 1, Minutes: 45}, UnitHour, UnitHour, 1, RoundHalfExpand); got != (Duration{Hours: 2}) {
		t.Errorf("1h45m rounds to %+v", got)
	}
	if got := round(Duration{Hours: 1, Minutes: 45}, UnitHour, UnitHour, 1, RoundTrunc); got != (Duration{Hours: 1}) {
		t.Errorf("1h45m truncs to %+v", got)
	}
	if got := round(Duration{Minutes: -90}, UnitMinute, UnitMinute, 45, RoundHalfExpand); got != (Duration{Minutes: -90}) {
		t.Errorf("-90m by 45m = %+v", got)
	}
	if got := round(Duration{Seconds: 1, Milliseconds: 500}, UnitSecond, UnitSecond, 1, RoundHalfEven); got != (Duration{Seconds: 2}) {
		t.Errorf("1.5s halfEven = %+v", got)
	}
	if got := round(Duration{Seconds: 2, Milliseconds: 500}, UnitSecond, UnitSecond, 1, RoundHalfEven); got != (Duration{Seconds: 2}) {
		t.Errorf("2.5s halfEven = %+v", got)
	}
	if _, err := RoundDuration(Duration{Months: 1}, UnitMonth, 1, UnitMonth, RoundHalfExpand, nil); err == nil {
		t.Error("calendar units without relativeTo must fail")
	}
	tot, err := TotalDuration(Duration{Hours: 130, Minutes: 20}, UnitSecond, nil)
	if err != nil || tot.Cmp(big.NewRat(469200, 1)) != 0 {
		t.Errorf("total seconds = %v (%v)", tot, err)
	}
	tot, _ = TotalDuration(Duration{Minutes: 90}, UnitHour, nil)
	if tot.Cmp(big.NewRat(3, 2)) != 0 {
		t.Errorf("90m in hours = %v", tot)
	}
}

func TestRoundAndTotalWithPlainRelativeTo(t *testing.T) {
	rel := &RelativeTo{Date: Date{2020, 1, 1}}
	got, err := RoundDuration(Duration{Days: 400}, UnitYear, 1, UnitNanosecond, RoundHalfExpand, rel)
	if err != nil || got != (Duration{Years: 1, Months: 1, Days: 3}) {
		t.Errorf("400 days = %+v (%v)", got, err)
	}
	got, err = RoundDuration(Duration{Years: 1, Months: 5, Days: 20}, UnitYear, 1, UnitYear, RoundHalfExpand, rel)
	if err != nil || got != (Duration{Years: 1}) {
		t.Errorf("1y5m20d to years = %+v (%v)", got, err)
	}
	got, err = RoundDuration(Duration{Years: 1, Months: 5, Days: 20}, UnitYear, 1, UnitMonth, RoundHalfExpand, rel)
	if err != nil || got != (Duration{Years: 1, Months: 6}) {
		t.Errorf("1y5m20d to months = %+v (%v)", got, err)
	}
	got, err = RoundDuration(Duration{Days: 40}, UnitMonth, 1, UnitMonth, RoundTrunc, rel)
	if err != nil || got != (Duration{Months: 1}) {
		t.Errorf("40 days trunc to months = %+v (%v)", got, err)
	}
	tot, err := TotalDuration(Duration{Months: 1}, UnitDay, &RelativeTo{Date: Date{2020, 2, 1}})
	if err != nil || tot.Cmp(big.NewRat(29, 1)) != 0 {
		t.Errorf("1 month from 2020-02-01 in days = %v (%v)", tot, err)
	}
	// 45 days from 2020-01-01 reach 2020-02-15: one month plus 14 of Feb's 29 days.
	tot, err = TotalDuration(Duration{Days: 45}, UnitMonth, &RelativeTo{Date: Date{2020, 1, 1}})
	if err != nil || tot.Cmp(big.NewRat(43, 29)) != 0 {
		t.Errorf("45 days in months = %v (%v), want 43/29", tot, err)
	}
}

func TestAddDurations(t *testing.T) {
	got, err := AddDurations(Duration{Hours: 1, Minutes: 45}, Duration{Minutes: 30}, 1, nil)
	if err != nil || got != (Duration{Hours: 2, Minutes: 15}) {
		t.Errorf("1h45m + 30m = %+v (%v)", got, err)
	}
	got, err = AddDurations(Duration{Hours: 1}, Duration{Minutes: 90}, -1, nil)
	if err != nil || got != (Duration{Minutes: -30}) {
		t.Errorf("1h - 90m = %+v (%v)", got, err)
	}
	if _, err := AddDurations(Duration{Months: 1}, Duration{Days: 1}, 1, nil); err == nil {
		t.Error("calendar units need relativeTo")
	}
	rel := &RelativeTo{Date: Date{2020, 1, 31}}
	got, err = AddDurations(Duration{Months: 1}, Duration{Days: 1}, 1, rel)
	// 2020-01-31 + 1 month = 2020-02-29 (constrained), +1 day = 2020-03-01: 1 month 1 day from Jan 31.
	if err != nil || got != (Duration{Months: 1, Days: 1}) {
		t.Errorf("P1M + P1D from 2020-01-31 = %+v (%v)", got, err)
	}
}

func TestCompareDurations(t *testing.T) {
	if c, err := CompareDurations(Duration{Hours: 1}, Duration{Minutes: 60}, nil); err != nil || c != 0 {
		t.Errorf("1h vs 60m = %d (%v)", c, err)
	}
	if c, _ := CompareDurations(Duration{Hours: 25}, Duration{Days: 1}, nil); c != 1 {
		t.Errorf("25h vs 1d = %d", c)
	}
	if _, err := CompareDurations(Duration{Months: 1}, Duration{Days: 30}, nil); err == nil {
		t.Error("calendar compare needs relativeTo")
	}
	rel := &RelativeTo{Date: Date{2020, 2, 1}}
	if c, _ := CompareDurations(Duration{Months: 1}, Duration{Days: 29}, rel); c != 0 {
		t.Errorf("1 month vs 29 days from 2020-02-01 = %d", c)
	}
	if c, _ := CompareDurations(Duration{Months: 1}, Duration{Days: 30}, rel); c != -1 {
		t.Errorf("1 month vs 30 days from 2020-02-01 = %d", c)
	}
}

func TestDifferenceISODateTime(t *testing.T) {
	a := DateTime{Date{2020, 1, 31}, Time{12, 0, 0, 0, 0, 0}}
	b := DateTime{Date{2020, 3, 1}, Time{11, 0, 0, 0, 0, 0}}
	got, err := DifferenceISODateTime(a, b, UnitMonth)
	if err != nil {
		t.Fatal(err)
	}
	// Time part is negative against a positive date part, so a day is borrowed.
	if got.Date != (DateDuration{0, 1, 0, 0}) || got.Time.Cmp(big.NewInt(23*3600*1e9)) != 0 {
		t.Errorf("diff = %+v time %v", got.Date, got.Time)
	}
	d, _ := got.ToDuration(UnitMonth)
	if d != (Duration{Months: 1, Hours: 23}) {
		t.Errorf("as duration = %+v", d)
	}
	// Not the mirror image: month-end clamping makes the difference
	// asymmetric, which is why since() is defined by negating until().
	rev, _ := DifferenceISODateTime(b, a, UnitMonth)
	if dr, _ := rev.ToDuration(UnitMonth); dr != (Duration{Days: -29, Hours: -23}) {
		t.Errorf("reverse = %+v", dr)
	}
	hours, _ := DifferenceISODateTime(a, b, UnitHour)
	if dh, _ := hours.ToDuration(UnitHour); dh.Hours != 719 || dh.Days != 0 {
		t.Errorf("in hours = %+v", dh)
	}
}

func TestRoundISODateTimeAndTime(t *testing.T) {
	dt := DateTime{Date{2020, 12, 31}, Time{23, 45, 0, 0, 0, 0}}
	got, err := RoundISODateTime(dt, 1, UnitHour, RoundHalfExpand)
	if err != nil || got != (DateTime{Date{2021, 1, 1}, Time{}}) {
		t.Errorf("round up across year = %+v (%v)", got, err)
	}
	days, tm := RoundTime(Time{12, 30, 0, 0, 0, 0}, 1, UnitHour, RoundHalfEven)
	if days != 0 || tm != (Time{12, 0, 0, 0, 0, 0}) {
		t.Errorf("12:30 halfEven = %d %+v", days, tm)
	}
	last := DateTime{Date{275760, 9, 13}, Time{23, 59, 59, 999, 999, 999}}
	if _, err := RoundISODateTime(last, 1, UnitDay, RoundFloor); err != nil {
		t.Errorf("rounding down inside the window: %v", err)
	}
	// Rounding up reaches +275760-09-14T00:00, the exclusive bound.
	if _, err := RoundISODateTime(last, 1, UnitDay, RoundCeil); err == nil {
		t.Error("rounding up to the exclusive bound must fail")
	}
}

func TestZoneDifferencesWithUTC(t *testing.T) {
	ns1, _ := new(big.Int).SetString("1577836800000000000", 10) // 2020-01-01T00:00:00Z
	ns2 := new(big.Int).Add(ns1, new(big.Int).Mul(big.NewInt(400), bigNsPerDay))
	ns2.Add(ns2, big.NewInt(3600*1e9)) // + 1h
	d, err := DifferenceZonedDateTime(ns1, ns2, utcZone{}, UnitYear)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := d.ToDuration(UnitYear)
	if got != (Duration{Years: 1, Months: 1, Days: 3, Hours: 1}) {
		t.Errorf("zoned diff = %+v", got)
	}
	r, err := DifferenceZonedDateTimeWithRounding(ns1, ns2, utcZone{}, UnitYear, 1, UnitMonth, RoundHalfExpand)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := r.ToDuration(UnitYear); got != (Duration{Years: 1, Months: 1}) {
		t.Errorf("zoned rounded diff = %+v", got)
	}
}

func TestRoundAsIfPositive(t *testing.T) {
	// A negative epoch time: "expand"/"ceil" go toward LATER times.
	x := big.NewInt(-1000000000000000000) // 1938-04-24T22:13:20Z in ns
	hour := big.NewInt(3600 * 1e9)
	down := RoundNumberToIncrementAsIfPositive(x, hour, RoundTrunc)
	up := RoundNumberToIncrementAsIfPositive(x, hour, RoundExpand)
	if down.String() != "-1000000800000000000" || up.String() != "-999997200000000000" {
		t.Errorf("trunc %v expand %v", down, up)
	}
	if got := RoundNumberToIncrementAsIfPositive(x, hour, RoundHalfExpand); got.Cmp(down) != 0 {
		t.Errorf("halfExpand = %v, want the nearer (earlier) hour", got)
	}
	// The ordinary version treats the sign as meaningful.
	if got := RoundNumberToIncrement(x, hour, RoundExpand); got.Cmp(down) != 0 {
		t.Errorf("signed expand = %v, want away from zero", got)
	}
}

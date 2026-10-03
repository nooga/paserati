package temporal

import (
	"math/big"
	"testing"
)

func TestEpochDays(t *testing.T) {
	cases := []struct {
		d    Date
		days int64
	}{
		{Date{1970, 1, 1}, 0},
		{Date{1970, 1, 2}, 1},
		{Date{1969, 12, 31}, -1},
		{Date{2000, 3, 1}, 11017},
		{Date{2024, 2, 29}, 19782},
		{Date{-271821, 4, 19}, minEpochDay},
		{Date{275760, 9, 13}, maxEpochDay},
		{Date{0, 1, 1}, -719528},
	}
	for _, c := range cases {
		if got := c.d.EpochDays(); got != c.days {
			t.Errorf("%v.EpochDays() = %d, want %d", c.d, got, c.days)
		}
		if got := EpochDaysToISODate(c.days); got != c.d {
			t.Errorf("EpochDaysToISODate(%d) = %v, want %v", c.days, got, c.d)
		}
	}
	// A wide round trip.
	for d := int64(minEpochDay); d <= maxEpochDay; d += 7919 {
		if EpochDaysToISODate(d).EpochDays() != d {
			t.Fatalf("round trip failed at %d", d)
		}
	}
}

func TestDayOfWeekAndWeeks(t *testing.T) {
	cases := []struct {
		d         Date
		dow, doy  int
		week, wyr int
	}{
		{Date{1970, 1, 1}, 4, 1, 1, 1970},
		{Date{2024, 12, 30}, 1, 365, 1, 2025},
		{Date{2021, 1, 3}, 7, 3, 53, 2020},
		{Date{2020, 12, 31}, 4, 366, 53, 2020},
		{Date{2016, 1, 1}, 5, 1, 53, 2015},
		{Date{2019, 6, 15}, 6, 166, 24, 2019},
	}
	for _, c := range cases {
		if c.d.DayOfWeek() != c.dow || c.d.DayOfYear() != c.doy {
			t.Errorf("%v: dow=%d doy=%d, want %d %d", c.d, c.d.DayOfWeek(), c.d.DayOfYear(), c.dow, c.doy)
		}
		if w, y := c.d.WeekOfYear(); w != c.week || y != c.wyr {
			t.Errorf("%v: week %d/%d, want %d/%d", c.d, w, y, c.week, c.wyr)
		}
	}
}

func TestLimits(t *testing.T) {
	if !ISODateWithinLimits(Date{-271821, 4, 19}) || ISODateWithinLimits(Date{-271821, 4, 18}) {
		t.Error("lower date limit")
	}
	if !ISODateWithinLimits(Date{275760, 9, 13}) || ISODateWithinLimits(Date{275760, 9, 14}) {
		t.Error("upper date limit")
	}
	if !ISODateTimeWithinLimits(DateTime{Date{-271821, 4, 19}, Time{0, 0, 0, 0, 0, 1}}) {
		t.Error("just inside the lower datetime limit")
	}
	if ISODateTimeWithinLimits(DateTime{Date{-271821, 4, 19}, Time{}}) {
		t.Error("exactly the lower datetime bound is outside")
	}
	if !ISODateTimeWithinLimits(DateTime{Date{275760, 9, 13}, Time{23, 59, 59, 999, 999, 999}}) {
		t.Error("end of the upper date is inside")
	}
	if ISODateTimeWithinLimits(DateTime{Date{275760, 9, 14}, Time{}}) {
		t.Error("exactly the upper datetime bound is outside")
	}
}

func TestEpochNsRoundTrip(t *testing.T) {
	for _, ns := range []string{"0", "-1", "86399999999999", "-86400000000000", "8640000000000000000000", "-8640000000000000000000"} {
		n, _ := new(big.Int).SetString(ns, 10)
		if got := GetUTCEpochNanoseconds(EpochNsToDateTime(n)); got.Cmp(n) != 0 {
			t.Errorf("%s -> %v", ns, got)
		}
	}
	if dt := EpochNsToDateTime(big.NewInt(-1)); dt != (DateTime{Date{1969, 12, 31}, Time{23, 59, 59, 999, 999, 999}}) {
		t.Errorf("-1ns = %+v", dt)
	}
}

func TestAddISODate(t *testing.T) {
	cases := []struct {
		d           Date
		y, m, w, dd int64
		overflow    Overflow
		want        Date
		wantErr     bool
	}{
		{Date{2020, 1, 31}, 0, 1, 0, 0, OverflowConstrain, Date{2020, 2, 29}, false},
		{Date{2020, 1, 31}, 0, 1, 0, 0, OverflowReject, Date{}, true},
		{Date{2020, 2, 29}, 1, 0, 0, 0, OverflowConstrain, Date{2021, 2, 28}, false},
		{Date{2020, 12, 25}, 0, 0, 1, 10, OverflowConstrain, Date{2021, 1, 11}, false},
		{Date{2020, 3, 31}, 0, -1, 0, 0, OverflowConstrain, Date{2020, 2, 29}, false},
		{Date{2020, 1, 1}, 0, 0, 0, -1, OverflowConstrain, Date{2019, 12, 31}, false},
		{Date{275760, 9, 13}, 0, 0, 0, 1, OverflowConstrain, Date{}, true},
	}
	for _, c := range cases {
		got, err := AddISODate(c.d, c.y, c.m, c.w, c.dd, c.overflow)
		if (err != nil) != c.wantErr || (err == nil && got != c.want) {
			t.Errorf("AddISODate(%v,%d,%d,%d,%d) = %v, %v; want %v (err %v)", c.d, c.y, c.m, c.w, c.dd, got, err, c.want, c.wantErr)
		}
	}
}

func TestDifferenceISODate(t *testing.T) {
	cases := []struct {
		one, two Date
		unit     Unit
		want     DateDuration
	}{
		{Date{2020, 1, 31}, Date{2020, 3, 1}, UnitMonth, DateDuration{0, 1, 0, 1}},
		{Date{2020, 1, 31}, Date{2020, 3, 1}, UnitDay, DateDuration{0, 0, 0, 30}},
		{Date{2020, 1, 1}, Date{2021, 3, 5}, UnitYear, DateDuration{1, 2, 0, 4}},
		{Date{2020, 1, 1}, Date{2021, 3, 5}, UnitMonth, DateDuration{0, 14, 0, 4}},
		{Date{2020, 1, 1}, Date{2021, 3, 5}, UnitWeek, DateDuration{0, 0, 61, 2}},
		{Date{2021, 3, 5}, Date{2020, 1, 1}, UnitYear, DateDuration{-1, -2, 0, -4}},
		{Date{2020, 2, 29}, Date{2021, 2, 28}, UnitYear, DateDuration{1, 0, 0, 0}},
		{Date{2020, 1, 31}, Date{2020, 2, 29}, UnitMonth, DateDuration{0, 1, 0, 0}},
		{Date{2021, 2, 28}, Date{2020, 2, 29}, UnitYear, DateDuration{0, -11, 0, -28}},
		{Date{2020, 2, 29}, Date{2024, 2, 29}, UnitYear, DateDuration{4, 0, 0, 0}},
		{Date{2020, 5, 5}, Date{2020, 5, 5}, UnitYear, DateDuration{}},
		{Date{-271821, 4, 19}, Date{275760, 9, 13}, UnitYear, DateDuration{547581, 4, 0, 25}},
	}
	for _, c := range cases {
		if got := DifferenceISODate(c.one, c.two, c.unit); got != c.want {
			t.Errorf("DifferenceISODate(%v, %v, %v) = %+v, want %+v", c.one, c.two, c.unit, got, c.want)
		}
	}
	// Adding the difference back reproduces the target (reject mode can fail
	// on month-end clamping, so use constrain).
	for _, c := range cases {
		d := DifferenceISODate(c.one, c.two, c.unit)
		back, err := AddISODate(c.one, d.Years, d.Months, d.Weeks, d.Days, OverflowConstrain)
		if err != nil || back != c.two {
			t.Errorf("%v + %+v = %v (%v), want %v", c.one, d, back, err, c.two)
		}
	}
}

func TestBalance(t *testing.T) {
	if d := BalanceISODate(2020, 14, 35); d != (Date{2021, 3, 7}) {
		t.Errorf("BalanceISODate = %v", d)
	}
	if y, m := BalanceISOYearMonth(2020, 0); y != 2019 || m != 12 {
		t.Errorf("BalanceISOYearMonth(2020,0) = %d-%d", y, m)
	}
	if y, m := BalanceISOYearMonth(2020, -13); y != 2018 || m != 11 {
		t.Errorf("BalanceISOYearMonth(2020,-13) = %d-%d", y, m)
	}
	days, tm := BalanceTime(big.NewInt(-1))
	if days != -1 || tm != (Time{23, 59, 59, 999, 999, 999}) {
		t.Errorf("BalanceTime(-1) = %d %+v", days, tm)
	}
}

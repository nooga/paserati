package temporal

import "testing"

func TestFormatYear(t *testing.T) {
	for in, want := range map[int]string{0: "0000", 1: "0001", 1999: "1999", 9999: "9999", 10000: "+010000", 275760: "+275760", -1: "-000001", -271821: "-271821", 12345: "+012345"} {
		if got := FormatYear(in); got != want {
			t.Errorf("FormatYear(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatDateTime(t *testing.T) {
	if got := FormatDate(Date{2020, 2, 9}); got != "2020-02-09" {
		t.Error(got)
	}
	if got := FormatDate(Date{-271821, 4, 19}); got != "-271821-04-19" {
		t.Error(got)
	}
	tm := Time{1, 2, 3, 400, 500, 600}
	for _, tt := range []struct {
		t    Time
		p    int
		want string
	}{
		{tm, -1, "01:02:03.4005006"},
		{tm, -2, "01:02"},
		{tm, 0, "01:02:03"},
		{tm, 1, "01:02:03.4"},
		{tm, 3, "01:02:03.400"},
		{tm, 9, "01:02:03.400500600"},
		{Time{1, 2, 3, 0, 0, 0}, -1, "01:02:03"},
		{Time{1, 2, 3, 0, 0, 0}, 3, "01:02:03.000"},
		{Time{1, 2, 3, 0, 0, 1}, -1, "01:02:03.000000001"},
		{Time{}, -1, "00:00:00"},
		{Time{23, 59, 59, 999, 999, 999}, 9, "23:59:59.999999999"},
		{Time{23, 59, 59, 999, 999, 999}, -2, "23:59"},
	} {
		if got := FormatTime(tt.t, tt.p); got != tt.want {
			t.Errorf("FormatTime(%+v, %d) = %q, want %q", tt.t, tt.p, got, tt.want)
		}
	}
	dt := DateTime{Date{2020, 1, 1}, Time{12, 0, 0, 0, 0, 0}}
	if got := FormatDateTime(dt, -1); got != "2020-01-01T12:00:00" {
		t.Error(got)
	}
	if got := FormatDateTime(dt, -2); got != "2020-01-01T12:00" {
		t.Error(got)
	}
}

func TestFormatOffset(t *testing.T) {
	const m, s = int64(60e9), int64(1e9)
	for _, tt := range []struct {
		ns    int64
		round bool
		want  string
	}{
		{0, false, "+00:00"},
		{0, true, "+00:00"},
		{3600 * s, false, "+01:00"},
		{-(8*60 + 30) * m, false, "-08:30"},
		{3600*s + 30*s, false, "+01:00:30"},
		{3600*s + 30*s, true, "+01:01"},
		{3600*s + 29*s + 999999999, true, "+01:00"},
		{-(3600*s + 30*s), true, "-01:01"},
		{-(3600*s + 29*s), true, "-01:00"},
		{3600*s + 500000000, false, "+01:00:00.5"},
		{-(30*s + 123456789), false, "-00:00:30.123456789"},
		{30 * s, true, "+00:01"},
		{29 * s, true, "+00:00"},
		{-30 * s, true, "-00:01"},
		{(23*3600 + 59*60) * s, false, "+23:59"},
		{1, false, "+00:00:00.000000001"},
	} {
		if got := FormatOffset(tt.ns, tt.round); got != tt.want {
			t.Errorf("FormatOffset(%d, %v) = %q, want %q", tt.ns, tt.round, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for _, tt := range []struct {
		d    Duration
		p    int
		want string
	}{
		{Duration{}, -1, "PT0S"},
		{Duration{}, 0, "PT0S"},
		{Duration{}, 3, "PT0.000S"},
		{Duration{Years: 1}, -1, "P1Y"},
		{Duration{Years: 1, Months: 2, Weeks: 3, Days: 4}, -1, "P1Y2M3W4D"},
		{Duration{Years: -1, Months: -2, Weeks: -3, Days: -4}, -1, "-P1Y2M3W4D"},
		{Duration{Hours: 5, Minutes: 6, Seconds: 7}, -1, "PT5H6M7S"},
		{Duration{Days: 1, Hours: 1}, -1, "P1DT1H"},
		{Duration{Minutes: 5}, -1, "PT5M"},
		{Duration{Days: 1}, 2, "P1DT0.00S"},
		{Duration{Years: 1, Months: 1, Weeks: 1, Days: 1, Hours: 1, Minutes: 1, Seconds: 1, Milliseconds: 1, Microseconds: 1, Nanoseconds: 1}, -1, "P1Y1M1W1DT1H1M1.001001001S"},
		{Duration{Seconds: -1, Milliseconds: -500}, -1, "-PT1.5S"},
		{Duration{Milliseconds: 1500}, -1, "PT1.5S"},
		{Duration{Milliseconds: 1}, -1, "PT0.001S"},
		{Duration{Nanoseconds: 1}, -1, "PT0.000000001S"},
		{Duration{Nanoseconds: 1e9}, -1, "PT1S"},
		{Duration{Microseconds: 1e6 + 1}, -1, "PT1.000001S"},
		{Duration{Seconds: 1, Milliseconds: 500}, 0, "PT1S"},
		{Duration{Seconds: 1, Milliseconds: 500}, 1, "PT1.5S"},
		{Duration{Seconds: 1, Milliseconds: 500}, 3, "PT1.500S"},
		{Duration{Seconds: 1, Milliseconds: 500}, 9, "PT1.500000000S"},
		{Duration{Seconds: 1}, 9, "PT1.000000000S"},
		{Duration{Hours: 1}, 1, "PT1H0.0S"},
		{Duration{Seconds: 9007199254740991}, -1, "PT9007199254740991S"},
		{Duration{Seconds: 9007199254740991, Milliseconds: 999, Microseconds: 999, Nanoseconds: 999}, -1, "PT9007199254740991.999999999S"},
		{Duration{Seconds: 9007199254740991, Milliseconds: 1000}, -1, "PT9007199254740992S"},
		{Duration{Milliseconds: 9007199254740991}, -1, "PT9007199254740.991S"},
		{Duration{Nanoseconds: -9007199254740991}, -1, "-PT9007199.254740991S"},
		{Duration{Years: 4294967295, Hours: 9007199254740991}, -1, "P4294967295YT9007199254740991H"},
		{Duration{Minutes: -90}, -1, "-PT90M"},
	} {
		if got := FormatDuration(tt.d, tt.p); got != tt.want {
			t.Errorf("FormatDuration(%+v, %d) = %q, want %q", tt.d, tt.p, got, tt.want)
		}
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	for _, in := range []string{"P1Y2M3W4DT5H6M7.008009001S", "-PT1.5S", "PT0S", "P1D", "-P1Y", "PT36H", "PT0.000000001S"} {
		d, err := ParseDurationString(in)
		if err != nil {
			t.Fatal(in, err)
		}
		if got := FormatDuration(d, -1); got != in {
			t.Errorf("%q round-tripped to %q", in, got)
		}
	}
}

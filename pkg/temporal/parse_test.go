package temporal

import (
	"errors"
	"reflect"
	"testing"
)

type parseFn func(string) (*Parsed, error)

func TestParseValid(t *testing.T) {
	tests := []struct {
		name string
		fn   parseFn
		in   string
		want Parsed
	}{
		{"date", ParsePlainDateString, "2020-01-01", Parsed{HasDate: true, Date: Date{2020, 1, 1}}},
		{"date compact", ParsePlainDateString, "20200229", Parsed{HasDate: true, Date: Date{2020, 2, 29}}},
		{"date ext year", ParsePlainDateString, "+002020-12-31", Parsed{HasDate: true, Date: Date{2020, 12, 31}}},
		{"date neg year", ParsePlainDateString, "-000001-01-01", Parsed{HasDate: true, Date: Date{-1, 1, 1}}},
		{"date ext compact", ParsePlainDateString, "+0020200101", Parsed{HasDate: true, Date: Date{2020, 1, 1}}},
		{"date with time ignored", ParsePlainDateString, "2020-01-01T12:34", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{Hour: 12, Minute: 34}}},
		{"date lowercase t", ParsePlainDateString, "2020-01-01t01", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{Hour: 1}}},
		{"date space sep", ParsePlainDateString, "2020-01-01 01:02:03", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{1, 2, 3, 0, 0, 0}}},
		{"datetime fraction", ParsePlainDateTimeString, "2020-01-01T01:02:03.123456789", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{1, 2, 3, 123, 456, 789}}},
		{"datetime comma fraction", ParsePlainDateTimeString, "2020-01-01T01:02:03,5", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{1, 2, 3, 500, 0, 0}}},
		{"datetime compact", ParsePlainDateTimeString, "20200101T010203.04", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{1, 2, 3, 40, 0, 0}}},
		{"datetime hhmm", ParsePlainDateTimeString, "2020-01-01T0102", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{Hour: 1, Minute: 2}}},
		{"leap second", ParsePlainDateTimeString, "2016-12-31T23:59:60", Parsed{HasDate: true, Date: Date{2016, 12, 31}, HasTime: true, Time: Time{23, 59, 59, 0, 0, 0}}},
		{"datetime offset ignored", ParsePlainDateTimeString, "2020-01-01T00:00+01:00", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, HasOffset: true, OffsetNs: 3600e9}},
		{"annotations", ParsePlainDateTimeString, "2020-01-01T00:00[UTC][foo=bar][u-ca=iso8601]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, TimeZone: "UTC", Calendar: "iso8601"}},
		{"critical iso8601", ParsePlainDateString, "2020-01-01[!u-ca=iso8601]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, Calendar: "iso8601", CalendarCritical: true}},
		{"unknown non-critical", ParsePlainDateString, "2020-01-01[foo=bar]", Parsed{HasDate: true, Date: Date{2020, 1, 1}}},
		{"multiple calendars", ParsePlainDateString, "2020-01-01[u-ca=iso8601][u-ca=gregory]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, Calendar: "iso8601"}},
		{"calendar value case", ParsePlainDateString, "2020-01-01[u-ca=ISO8601]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, Calendar: "ISO8601"}},
		{"offset tz annotation", ParsePlainDateString, "2020-01-01[+01:00]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, TimeZone: "+01:00"}},
		{"critical tz", ParsePlainDateString, "2020-01-01[!America/New_York]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, TimeZone: "America/New_York", TimeZoneCritical: true}},

		{"instant Z", ParseInstantString, "1970-01-01T00:00Z", Parsed{HasDate: true, Date: Date{1970, 1, 1}, HasTime: true, UTC: true}},
		{"instant lowercase z", ParseInstantString, "1970-01-01T00:00:00z", Parsed{HasDate: true, Date: Date{1970, 1, 1}, HasTime: true, UTC: true}},
		{"instant offset", ParseInstantString, "1970-01-01T00:00-08:30", Parsed{HasDate: true, Date: Date{1970, 1, 1}, HasTime: true, HasOffset: true, OffsetNs: -(8*3600 + 30*60) * 1e9}},
		{"instant subminute", ParseInstantString, "1970-01-01T00:00+01:00:30.5", Parsed{HasDate: true, Date: Date{1970, 1, 1}, HasTime: true, HasOffset: true, OffsetNs: 3630.5e9, OffsetHasSubMinute: true}},
		{"instant compact offset", ParseInstantString, "1970-01-01T00:00+0100", Parsed{HasDate: true, Date: Date{1970, 1, 1}, HasTime: true, HasOffset: true, OffsetNs: 3600e9}},
		{"instant with tz", ParseInstantString, "1970-01-01T00:00Z[Europe/Paris]", Parsed{HasDate: true, Date: Date{1970, 1, 1}, HasTime: true, UTC: true, TimeZone: "Europe/Paris"}},

		{"zdt", ParseZonedDateTimeString, "2020-01-01T00:00[UTC]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, TimeZone: "UTC"}},
		{"zdt no time", ParseZonedDateTimeString, "2020-01-01[UTC]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, TimeZone: "UTC"}},
		{"zdt Z", ParseZonedDateTimeString, "2020-01-01T00:00Z[UTC]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, UTC: true, TimeZone: "UTC"}},
		{"zdt offset zone", ParseZonedDateTimeString, "2020-01-01T00:00+01:00[+01:00]", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, HasOffset: true, OffsetNs: 3600e9, TimeZone: "+01:00"}},

		{"time bare", ParsePlainTimeString, "12:34:56.789", Parsed{HasTime: true, Time: Time{12, 34, 56, 789, 0, 0}}},
		{"time T", ParsePlainTimeString, "T12", Parsed{HasTime: true, Time: Time{Hour: 12}}},
		{"time t compact", ParsePlainTimeString, "t1234", Parsed{HasTime: true, Time: Time{Hour: 12, Minute: 34}}},
		{"time hh", ParsePlainTimeString, "12", Parsed{HasTime: true, Time: Time{Hour: 12}}},
		{"time hhmm", ParsePlainTimeString, "2021", Parsed{HasTime: true, Time: Time{Hour: 20, Minute: 21}}},
		{"time with offset", ParsePlainTimeString, "12:34+01:00[UTC]", Parsed{HasTime: true, Time: Time{Hour: 12, Minute: 34}, HasOffset: true, OffsetNs: 3600e9, TimeZone: "UTC"}},
		{"time from datetime", ParsePlainTimeString, "2020-01-01T12:34", Parsed{HasDate: true, Date: Date{2020, 1, 1}, HasTime: true, Time: Time{Hour: 12, Minute: 34}}},
		{"time leap", ParsePlainTimeString, "23:59:60", Parsed{HasTime: true, Time: Time{23, 59, 59, 0, 0, 0}}},
		{"time T disambiguates", ParsePlainTimeString, "T1214", Parsed{HasTime: true, Time: Time{Hour: 12, Minute: 14}}},
		{"time month 13 not a date", ParsePlainTimeString, "1314", Parsed{HasTime: true, Time: Time{Hour: 13, Minute: 14}}},
		{"time annotation", ParsePlainTimeString, "12:00[u-ca=iso8601]", Parsed{HasTime: true, Time: Time{Hour: 12}, Calendar: "iso8601"}},

		{"ym short", ParsePlainYearMonthString, "2021-12", Parsed{HasDate: true, Date: Date{2021, 12, 1}}},
		{"ym compact", ParsePlainYearMonthString, "202112", Parsed{HasDate: true, Date: Date{2021, 12, 1}}},
		{"ym ext", ParsePlainYearMonthString, "+002021-12", Parsed{HasDate: true, Date: Date{2021, 12, 1}}},
		{"ym full", ParsePlainYearMonthString, "2021-12-14T10:00", Parsed{HasDate: true, Date: Date{2021, 12, 14}, HasTime: true, Time: Time{Hour: 10}}},
		{"ym annotation", ParsePlainYearMonthString, "2021-12[u-ca=iso8601]", Parsed{HasDate: true, Date: Date{2021, 12, 1}, Calendar: "iso8601"}},
		{"md --", ParsePlainMonthDayString, "--12-14", Parsed{HasDate: true, Date: Date{1972, 12, 14}}},
		{"md --compact", ParsePlainMonthDayString, "--1214", Parsed{HasDate: true, Date: Date{1972, 12, 14}}},
		{"md bare", ParsePlainMonthDayString, "12-14", Parsed{HasDate: true, Date: Date{1972, 12, 14}}},
		{"md compact", ParsePlainMonthDayString, "1214", Parsed{HasDate: true, Date: Date{1972, 12, 14}}},
		{"md leap day", ParsePlainMonthDayString, "02-29", Parsed{HasDate: true, Date: Date{1972, 2, 29}}},
		{"md full", ParsePlainMonthDayString, "2021-12-14", Parsed{HasDate: true, Date: Date{2021, 12, 14}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fn(tt.in)
			if err != nil {
				t.Fatalf("%q: unexpected error %v", tt.in, err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("%q:\n got  %+v\n want %+v", tt.in, *got, tt.want)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	common := []string{
		"", "invalid iso8601", "2020-01-00", "2020-01-32", "2020-02-30", "2021-02-29", "2020-00-01", "2020-13-01",
		"2020-01-01T", "2020-01-01T25:00:00", "2020-01-01T01:60:00", "2020-01-01T01:60:61", "2020-01-01junk",
		"2020-01-01T00:00:00junk", "2020-01-01T00:00:00+00:00junk", "2020-01-01T00:00:00+00:00[UTC]junk",
		"2020-01-01T00:00:00+00:00[UTC][u-ca=iso8601]junk", "02020-01-01", "2020-001-01", "2020-01-001",
		"2020-01-01T001", "2020-01-01T01:001", "2020-01-01T01:01:001", "2020-W01-1", "2020-001",
		"+0002020-01-01", "2020-0101", "202001-01", "-000000-01-01", "+0000000-01-01", "\u2212000000-01-01",
		"2020-01-01T01:02:03.", "2020-01-01T01:02:03.1234567890", "2020-01-01T01.5", "2020-01-01T01:02.5",
		"2020-01-01T01:0203", "2020-01-01T0102:03", "2020-01-01T24:00", "2020-01-01T00:00+24:00",
		"2020-01-01T00:00-00:60", "2020-01-01T00:00+00:00:60", "2020-01-01T00:00+00:0000", "2020-01-01T00:00+0000:00",
		"2020-01-01T00:00+1", "2020-01-01+01:00", "2020-01-01Z", "2020-01-01T00:00 [UTC]", "2020-01-01  00:00",
		"2020-01-01T00:00[]", "2020-01-01T00:00[UTC", "2020-01-01T00:00[UTC][Europe/Paris]",
		"2020-01-01T00:00[u-ca=iso8601][UTC]", "2020-01-01T00:00[!foo=bar]", "2020-01-01[foo=bar][!baz=quux]",
		"2020-01-01[u-ca=iso8601][!u-ca=gregory]", "2020-01-01[!u-ca=iso8601][u-ca=iso8601]",
		"2020-01-01[Foo=bar]", "2020-01-01[foo=bar_baz]", "2020-01-01[foo=bar-]", "2020-01-01[foo=]",
		"2020-01-01[=bar]", "2020-01-01[foo=bar=baz]", "2020-01-01[UTC=]", "2020-01-01[+1:00]", "2020-01-01[+01:00:00]",
		"2020-01-01[/UTC]", "2020-01-01[Europe//Paris]", "2020-01-01[..]", "2020-01-01[Europe/.]", "2020-01-01[1UTC]",
		"2020-01-01[Averyveryverylongcomponent]", "P1Y", "-P12Y", "2020-01", "01-01", "--01-01",
	}
	fns := map[string]parseFn{
		"PlainDate": ParsePlainDateString, "PlainDateTime": ParsePlainDateTimeString,
		"ZonedDateTime": ParseZonedDateTimeString, "Instant": ParseInstantString,
	}
	for name, fn := range fns {
		for _, in := range common {
			if p, err := fn(in); err == nil {
				t.Errorf("%s: %q accepted: %+v", name, in, *p)
			} else {
				var re *RangeError
				if !errors.As(err, &re) {
					t.Errorf("%s: %q: error %T is not *RangeError", name, in, err)
				}
			}
		}
	}

	only := []struct {
		name string
		fn   parseFn
		in   []string
	}{
		{"PlainDate", ParsePlainDateString, []string{"2020-01-01T00:00Z", "2020-01-01T00:00:00z[UTC]"}},
		{"PlainDateTime", ParsePlainDateTimeString, []string{"2020-01-01T00:00Z", "2020-01-01T00Z[UTC]"}},
		{"Instant", ParseInstantString, []string{"2020-01-01", "2020-01-01T00:00", "2020-01-01T00:00[UTC]", "2020-01-01[UTC]"}},
		{"ZonedDateTime", ParseZonedDateTimeString, []string{"2020-01-01T00:00", "2020-01-01T00:00Z", "2020-01-01T00:00+01:00", "2020-01-01"}},
		{"PlainTime", ParsePlainTimeString, []string{
			"", "invalid iso8601", "00:00Z", "Z", "25:00:00Z", "01:60:00Z", "00:00Zjunk", "00:00:00+00:00junk",
			"00:00:00+00:00[UTC]junk", "001Z", "01:001Z", "00:00-24:00", "00:00+24:00", "00:00:00+00:0000",
			"0000:00", "00:0000", "00:00:00+0000:00", "T", "2020-01-01", "2020-01-01T", "2020-01-01T00:00Z",
			// ambiguous with a date, so a T prefix is required
			"2021-12", "202112", "1214", "12-14", "--12-14", "--1214", "0229", "1214[UTC]",
			"12:34[foo=bar][!baz=quux]", "12:34[u-ca=gregory][!u-ca=iso8601]", "T12:34[Foo/Bar][UTC]",
			"12:34:56.", "12:34.5", "1234:56", "12:3456", "TT12", "12:34 ", " 12:34",
		}},
		{"PlainYearMonth", ParsePlainYearMonthString, []string{
			"", "2021-13", "2021-00", "202113", "2021-1", "21-12", "2021-12-", "2021-12Z", "2021-12-14Z", "2021-12-14T00:00Z",
			"--12-14", "12-14", "2021-12[!foo=bar]", "2021-12junk", "-000000-12", "+2021-12", "2021--12", "2021 12",
		}},
		{"PlainMonthDay", ParsePlainMonthDayString, []string{
			"", "13-01", "00-01", "01-00", "01-32", "02-30", "04-31", "--1-1", "-12-14", "---12-14", "--12--14",
			"12-14Z", "2021-12-14Z", "12-14[!foo=bar]", "12-14junk", "2021-02-29", "12-14T00:00", "2021-12",
		}},
	}
	for _, o := range only {
		for _, in := range o.in {
			if p, err := o.fn(in); err == nil {
				t.Errorf("%s: %q accepted: %+v", o.name, in, *p)
			}
		}
	}
}

func TestParseDurationString(t *testing.T) {
	valid := []struct {
		in   string
		want Duration
	}{
		{"P1Y2M3W4DT5H6M7S", Duration{1, 2, 3, 4, 5, 6, 7, 0, 0, 0}},
		{"p1y2m3w4dt5h6m7s", Duration{1, 2, 3, 4, 5, 6, 7, 0, 0, 0}},
		{"-P1Y2M3W4DT5H6M7S", Duration{-1, -2, -3, -4, -5, -6, -7, 0, 0, 0}},
		{"+P1Y", Duration{Years: 1}},
		{"P1D", Duration{Days: 1}},
		{"PT1H", Duration{Hours: 1}},
		{"PT0S", Duration{}},
		{"-PT0S", Duration{}},
		{"P0D", Duration{}},
		{"PT0.5H", Duration{Minutes: 30}},
		{"PT0,5H", Duration{Minutes: 30}},
		{"PT1.5H", Duration{Hours: 1, Minutes: 30}},
		{"PT0.000000001H", Duration{Microseconds: 3, Nanoseconds: 600}},
		{"PT0.5M", Duration{Seconds: 30}},
		{"PT1.5S", Duration{Seconds: 1, Milliseconds: 500}},
		{"PT0.001002003S", Duration{Milliseconds: 1, Microseconds: 2, Nanoseconds: 3}},
		{"PT1H0.5M", Duration{Hours: 1, Seconds: 30}},
		{"P1Y1M1W1DT1H1M1.123456789S", Duration{1, 1, 1, 1, 1, 1, 1, 123, 456, 789}},
		{"-PT1.5S", Duration{Seconds: -1, Milliseconds: -500}},
		{"-PT0.5H", Duration{Minutes: -30}},
		{"PT9007199254740991S", Duration{Seconds: 9007199254740991}},
		{"P100000000000Y", Duration{Years: 1e11}},
		{"PT1H1M1S", Duration{Hours: 1, Minutes: 1, Seconds: 1}},
		{"P1DT1S", Duration{Days: 1, Seconds: 1}},
	}
	for _, tt := range valid {
		got, err := ParseDurationString(tt.in)
		if err != nil {
			t.Errorf("%q: unexpected error %v", tt.in, err)
		} else if got != tt.want {
			t.Errorf("%q: got %+v want %+v", tt.in, got, tt.want)
		}
	}
	invalid := []string{
		"", "P", "-P", "PT", "-PT", "+P", "P1DT", "1Y", "Y", "PY", "P1", "P1Y1", "PT1", "P-1Y", "P1Y-1M", "PT-1H", "--P1Y",
		"P1M1Y", "P1D1W", "PT1S1M", "PT1M1H", "P1Y1Y", "PT1H1H", "P1H", "PT1D", "P1S", "P2H", "P2S", "PT1Y",
		"P0.5Y", "P2.5M", "P1Y0,5M", "P1Y1M0.5W", "P1Y1M1W0,5D", "P1Y1M1W1DT0.5H5S", "P1Y1M1W1DT1.5H0,5M",
		"P1Y1M1W1DT1H0.5M0.5S", "P1Y1M1W1DT1H1M1.123456789123S", "P1Y1M1W1DT1H1M1.01Sjunk", "PT.1H", "PT.1S", "PT,1M",
		"PT1.S", "PT1.H", "PT2,H3M", "PT0.1H0S", "PT0.1M0.0S", "PT0.1H0M", "P1D T1H", " P1D", "P1D ", "PT1H junk",
		"P1YT", "PTT1H", "P1DTT1H", "PT1.5.5S", "PT1.5,5S", "P1W2", "PT\u0661S",
	}
	for _, in := range invalid {
		if got, err := ParseDurationString(in); err == nil {
			t.Errorf("%q accepted: %+v", in, got)
		}
	}
}

func TestParseTimeZoneIdentifier(t *testing.T) {
	offs := map[string]int64{
		"+01": 3600e9, "-01": -3600e9, "+01:00": 3600e9, "+0100": 3600e9, "-08:30": -(8*3600 + 30*60) * 1e9,
		"+23:59": (23*3600 + 59*60) * 1e9, "+00:00": 0, "-00:00": 0,
	}
	for in, want := range offs {
		ns, _, isOff, err := ParseTimeZoneIdentifier(in)
		if err != nil || !isOff || ns != want {
			t.Errorf("%q: got %d %v %v, want %d", in, ns, isOff, err, want)
		}
	}
	names := []string{"UTC", "Europe/Paris", "America/Argentina/Buenos_Aires", "Etc/GMT+1", "Etc/GMT-14", "EST5EDT", "America/Port-au-Prince", "Asia/Ho_Chi_Minh"}
	for _, in := range names {
		_, name, isOff, err := ParseTimeZoneIdentifier(in)
		if err != nil || isOff || name != in {
			t.Errorf("%q: got %q %v %v", in, name, isOff, err)
		}
	}
	bad := []string{
		"", "+", "+1", "+1:00", "+01:", "+01:0", "+01:00:00", "+0100:00", "+01:00:00.5", "+24:00", "-24:00", "+01:60", "+01000",
		"/", "UTC/", "/UTC", "A//B", ".", "..", "A/.", "1UTC", "-UTC", "UTC ", " UTC", "Europe/Pa ris", "Euro\u00e9/Paris", "Averyverylongnamex",
		"UTC[x]", "a=b", "+01:00 ",
	}
	for _, in := range bad {
		if _, _, _, err := ParseTimeZoneIdentifier(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestParseDateTimeUTCOffset(t *testing.T) {
	good := []struct {
		in   string
		ns   int64
		subM bool
	}{
		{"+01", 3600e9, false}, {"+01:00", 3600e9, false}, {"+0100", 3600e9, false}, {"-01:30", -5400e9, false},
		{"+01:00:30", 3630e9, true}, {"+010030", 3630e9, true}, {"-00:00:01.5", -1.5e9, true},
		{"+00:00:00.123456789", 123456789, true}, {"+00:00:00,5", 5e8, true}, {"-23:59:59.999999999", -(86399e9 + 999999999), true},
		{"+00:00:00", 0, true},
	}
	for _, tt := range good {
		ns, sub, err := ParseDateTimeUTCOffset(tt.in)
		if err != nil || ns != tt.ns || sub != tt.subM {
			t.Errorf("%q: got %d %v %v", tt.in, ns, sub, err)
		}
	}
	for _, in := range []string{"", "Z", "01:00", "+1", "+24:00", "+01:60", "+01:00:60", "+01:00:00.", "+01:00:00.1234567890", "+01:00.5",
		"+01:0000", "+0100:00", "+01:00:0", "+01:00junk", "+01:00[UTC]", " +01:00", "+01:00 "} {
		if _, _, err := ParseDateTimeUTCOffset(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestCanonicalizeCalendarIdentifier(t *testing.T) {
	for _, in := range []string{"iso8601", "ISO8601", "IsO8601"} {
		if c, ok := CanonicalizeCalendarIdentifier(in); !ok || c != "iso8601" {
			t.Errorf("%q: got %q %v", in, c, ok)
		}
	}
	for _, in := range []string{"", "gregory", "iso8601 ", "i\u017fo8601", "iso-8601", "\u212Aiso8601"} {
		if c, ok := CanonicalizeCalendarIdentifier(in); ok || c != "" {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestUnicodeMinusIsRejected(t *testing.T) {
	if _, err := ParsePlainDateString("−000001-01-01"); err == nil {
		t.Error("U+2212 year sign accepted")
	}
	if _, err := ParseInstantString("1976-11-18T15:23:30.12−02:00"); err == nil {
		t.Error("U+2212 offset sign accepted")
	}
	if _, err := ParseDurationString("−P1Y"); err == nil {
		t.Error("U+2212 duration sign accepted")
	}
}

func TestAnnotationPolicy(t *testing.T) {
	// Unknown annotations may have any alphanumeric value; the calendar
	// annotation is reported with its critical flag for the caller to judge.
	p, err := ParseInstantString("1970-01-01T00:00Z[foo=bar][_foo-bar0=Ignore-This-999999999999]")
	if err != nil || p.Calendar != "" {
		t.Errorf("unknown annotations: %v %+v", err, p)
	}
	p, err = ParseInstantString("1970-01-01T00:00Z[!u-ca=hebrew]")
	if err != nil || p.Calendar != "hebrew" || !p.CalendarCritical {
		t.Errorf("critical calendar: %v %+v", err, p)
	}
	if _, err := ParsePlainDateString("2020-01-01[!u-ca=iso8601][u-ca=iso8601]"); err == nil {
		t.Error("a critical calendar among several must be rejected")
	}
}

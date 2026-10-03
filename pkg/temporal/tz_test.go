package temporal

import (
	"errors"
	"math/big"
	"testing"
	"time"
)

func tzMust(t *testing.T, id string) TimeZone {
	t.Helper()
	tz, err := ParseTimeZone(id)
	if err != nil {
		t.Fatalf("ParseTimeZone(%q): %v", id, err)
	}
	return tz
}

func tzNs(y int, mo time.Month, d, h, mi, s int) *big.Int {
	return tzJoin(time.Date(y, mo, d, h, mi, s, 0, time.UTC).Unix(), 0)
}

func tzDT(y, mo, d, h, mi, s int) DateTime {
	return DateTime{Date{y, mo, d}, Time{Hour: h, Minute: mi, Second: s}}
}

func TestTimeZoneCanonicalization(t *testing.T) {
	for in, want := range map[string]string{
		"UTC": "UTC", "utc": "UTC", "uTc": "UTC", "Etc/UTC": "UTC", "etc/gmt": "UTC", "GMT": "UTC", "Zulu": "UTC",
		"europe/warsaw": "Europe/Warsaw", "AMERICA/NEW_YORK": "America/New_York", "asia/kolkata": "Asia/Kolkata",
		"asia/calcutta": "Asia/Calcutta", "etc/gmt+5": "Etc/GMT+5",
		"+01": "+01:00", "+0530": "+05:30", "-05:30": "-05:30", "+00:00": "+00:00", "-00:00": "+00:00",
		"+23:59": "+23:59", "-2359": "-23:59",
	} {
		if got := tzMust(t, in).ID(); got != want {
			t.Errorf("ParseTimeZone(%q).ID() = %q, want %q", in, got, want)
		}
	}
	if !tzMust(t, "+01").IsOffset() || tzMust(t, "UTC").IsOffset() {
		t.Error("IsOffset wrong")
	}
	if !tzMust(t, "+01:00").Equal(tzMust(t, "+0100")) || tzMust(t, "+01:00").Equal(tzMust(t, "UTC")) {
		t.Error("Equal wrong")
	}
}

func TestTimeZoneInvalid(t *testing.T) {
	for _, id := range []string{"", "+25:00", "+24:00", "+01:60", "+01:00:30", "+01:00:00.5", "Nowhere/Land", "UTC+1", "+1", "+0", "+01 00", "Europe/Warsaw ", "../etc/passwd", "−01:00", "+aa:bb"} {
		_, err := ParseTimeZone(id)
		var re *RangeError
		if !errors.As(err, &re) {
			t.Errorf("ParseTimeZone(%q) error = %v, want RangeError", id, err)
		}
	}
}

func TestSystemTimeZone(t *testing.T) {
	if SystemTimeZone().ID() == "" {
		t.Error("empty system zone")
	}
	t.Setenv("TZ", "")
}

func TestOffsets(t *testing.T) {
	const h, m = int64(3600e9), int64(60e9)
	for _, c := range []struct {
		zone string
		at   *big.Int
		want int64
	}{
		{"America/New_York", tzNs(2024, 3, 10, 6, 59, 59), -5 * h},
		{"America/New_York", tzNs(2024, 3, 10, 7, 0, 0), -4 * h},
		{"America/New_York", tzNs(2024, 11, 3, 5, 59, 59), -4 * h},
		{"America/New_York", tzNs(2024, 11, 3, 6, 0, 0), -5 * h},
		{"America/New_York", tzNs(1850, 1, 1, 0, 0, 0), -(4*h + 56*m + 2e9)},
		{"Europe/Warsaw", tzNs(2024, 7, 1, 0, 0, 0), 2 * h},
		{"Europe/Warsaw", tzNs(2024, 1, 1, 0, 0, 0), 1 * h},
		{"Asia/Kolkata", tzNs(2024, 7, 1, 0, 0, 0), 5*h + 30*m},
		{"Asia/Kolkata", tzNs(2024, 1, 1, 0, 0, 0), 5*h + 30*m},
		{"Pacific/Chatham", tzNs(2024, 1, 1, 0, 0, 0), 13*h + 45*m},
		{"Pacific/Chatham", tzNs(2024, 7, 1, 0, 0, 0), 12*h + 45*m},
		{"Australia/Lord_Howe", tzNs(2024, 1, 1, 0, 0, 0), 11 * h},
		{"Australia/Lord_Howe", tzNs(2024, 7, 1, 0, 0, 0), 10*h + 30*m},
		{"UTC", tzNs(2024, 7, 1, 0, 0, 0), 0},
		{"-08:30", tzNs(2024, 7, 1, 0, 0, 0), -(8*h + 30*m)},
		{"America/New_York", tzJoin(8.64e12, 0), -4 * h},
		{"America/New_York", tzJoin(-8.64e12, 0), -(4*h + 56*m + 2e9)},
	} {
		if got := tzMust(t, c.zone).OffsetNanosecondsAt(c.at); got != c.want {
			t.Errorf("%s offset at %v = %d, want %d", c.zone, c.at, got, c.want)
		}
	}
}

func TestLocalDateTime(t *testing.T) {
	ny := tzMust(t, "America/New_York")
	got := ny.LocalDateTime(new(big.Int).Add(tzNs(2024, 3, 10, 7, 0, 0), big.NewInt(123456789)))
	want := DateTime{Date{2024, 3, 10}, Time{3, 0, 0, 123, 456, 789}}
	if got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
	// Negative epoch values floor correctly: 1 ns before the epoch.
	if got, want := tzMust(t, "UTC").LocalDateTime(big.NewInt(-1)), (DateTime{Date{1969, 12, 31}, Time{23, 59, 59, 999, 999, 999}}); got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
	// LMT with sub-minute offset.
	if got, want := ny.LocalDateTime(tzNs(1850, 1, 1, 0, 0, 0)), tzDT(1849, 12, 31, 19, 3, 58); got != want {
		t.Errorf("got %+v want %+v", got, want)
	}
	// Extremes do not overflow.
	max := new(big.Int).Set(MaxEpochNanoseconds)
	if got := tzMust(t, "UTC").LocalDateTime(max).Date; got != (Date{275760, 9, 13}) {
		t.Errorf("max date %+v", got)
	}
	if got := tzMust(t, "UTC").LocalDateTime(max.Neg(max)).Date; got != (Date{-271821, 4, 20}) {
		t.Errorf("min date %+v", got)
	}
}

func TestPossibleAndDisambiguation(t *testing.T) {
	ny := tzMust(t, "America/New_York")
	gap := tzDT(2024, 3, 10, 2, 30, 0)     // skipped
	overlap := tzDT(2024, 11, 3, 1, 30, 0) // repeated
	plain := tzDT(2024, 6, 1, 12, 0, 0)

	if n := len(ny.PossibleEpochNanoseconds(gap)); n != 0 {
		t.Errorf("gap: %d candidates", n)
	}
	p := ny.PossibleEpochNanoseconds(overlap)
	if len(p) != 2 || p[0].Cmp(tzNs(2024, 11, 3, 5, 30, 0)) != 0 || p[1].Cmp(tzNs(2024, 11, 3, 6, 30, 0)) != 0 {
		t.Errorf("overlap: %v", p)
	}
	if p := ny.PossibleEpochNanoseconds(plain); len(p) != 1 || p[0].Cmp(tzNs(2024, 6, 1, 16, 0, 0)) != 0 {
		t.Errorf("plain: %v", p)
	}

	for _, c := range []struct {
		name  string
		local DateTime
		d     Disambiguation
		want  *big.Int // nil means RangeError
	}{
		{"gap compatible", gap, DisambiguateCompatible, tzNs(2024, 3, 10, 7, 30, 0)}, // 03:30 EDT
		{"gap later", gap, DisambiguateLater, tzNs(2024, 3, 10, 7, 30, 0)},
		{"gap earlier", gap, DisambiguateEarlier, tzNs(2024, 3, 10, 6, 30, 0)}, // 01:30 EST
		{"gap reject", gap, DisambiguateReject, nil},
		{"overlap compatible", overlap, DisambiguateCompatible, tzNs(2024, 11, 3, 5, 30, 0)},
		{"overlap earlier", overlap, DisambiguateEarlier, tzNs(2024, 11, 3, 5, 30, 0)},
		{"overlap later", overlap, DisambiguateLater, tzNs(2024, 11, 3, 6, 30, 0)},
		{"overlap reject", overlap, DisambiguateReject, nil},
		{"plain reject", plain, DisambiguateReject, tzNs(2024, 6, 1, 16, 0, 0)},
	} {
		got, err := ny.EpochNanosecondsFor(c.local, c.d)
		if c.want == nil {
			var re *RangeError
			if !errors.As(err, &re) {
				t.Errorf("%s: err = %v, want RangeError", c.name, err)
			}
		} else if err != nil || got.Cmp(c.want) != 0 {
			t.Errorf("%s: got %v, %v want %v", c.name, got, err, c.want)
		}
	}

	// A 30-minute gap (Lord Howe, 2024-10-06 02:00 -> 02:30).
	lh := tzMust(t, "Australia/Lord_Howe")
	got, err := lh.EpochNanosecondsFor(tzDT(2024, 10, 6, 2, 15, 0), DisambiguateCompatible)
	if err != nil || lh.LocalDateTime(got) != tzDT(2024, 10, 6, 2, 45, 0) {
		t.Errorf("lord howe gap: %v %v", got, err)
	}
	// Offset zones are never ambiguous.
	if p := tzMust(t, "+05:30").PossibleEpochNanoseconds(plain); len(p) != 1 || p[0].Cmp(tzNs(2024, 6, 1, 6, 30, 0)) != 0 {
		t.Errorf("offset zone: %v", p)
	}
	// Results outside the Instant range are rejected.
	if _, err := tzMust(t, "UTC").EpochNanosecondsFor(tzDT(275760, 9, 13, 0, 0, 1), DisambiguateCompatible); err == nil {
		t.Error("expected out-of-range error")
	}
	if _, err := tzMust(t, "UTC").EpochNanosecondsFor(tzDT(275760, 9, 13, 0, 0, 0), DisambiguateCompatible); err != nil {
		t.Errorf("max instant: %v", err)
	}
}

func TestTransitions(t *testing.T) {
	ny := tzMust(t, "America/New_York")
	start, end := tzNs(2024, 3, 10, 7, 0, 0), tzNs(2024, 11, 3, 6, 0, 0)

	if got, ok := ny.NextTransition(tzNs(2024, 1, 1, 0, 0, 0)); !ok || got.Cmp(start) != 0 {
		t.Errorf("next from jan: %v %v", got, ok)
	}
	if got, ok := ny.NextTransition(start); !ok || got.Cmp(end) != 0 {
		t.Errorf("next is strict: %v %v", got, ok)
	}
	if got, ok := ny.PreviousTransition(end); !ok || got.Cmp(start) != 0 {
		t.Errorf("previous is strict: %v %v", got, ok)
	}
	if got, ok := ny.PreviousTransition(new(big.Int).Add(end, big.NewInt(1))); !ok || got.Cmp(end) != 0 {
		t.Errorf("previous just after: %v %v", got, ok)
	}
	if got, ok := ny.PreviousTransition(start); !ok || got.Cmp(tzNs(2023, 11, 5, 6, 0, 0)) != 0 {
		t.Errorf("previous year: %v %v", got, ok)
	}

	// Before the first transition (NY's first is 1883-11-18 17:00 UTC).
	first := tzNs(1883, 11, 18, 17, 0, 0)
	if got, ok := ny.NextTransition(tzNs(1800, 1, 1, 0, 0, 0)); !ok || got.Cmp(first) != 0 {
		t.Errorf("first: %v %v", got, ok)
	}
	if got, ok := ny.PreviousTransition(first); ok {
		t.Errorf("before first: %v", got)
	}
	if _, ok := ny.PreviousTransition(tzNs(1800, 1, 1, 0, 0, 0)); ok {
		t.Error("previous in 1800")
	}

	// Zones with no DST end up with no further transitions (Kolkata, 1945).
	kol := tzMust(t, "Asia/Kolkata")
	if got, ok := kol.NextTransition(tzNs(2024, 1, 1, 0, 0, 0)); ok {
		t.Errorf("kolkata next: %v", got)
	}
	if _, ok := kol.PreviousTransition(tzNs(2024, 1, 1, 0, 0, 0)); !ok {
		t.Error("kolkata has past transitions")
	}

	// Offset zones and UTC never transition.
	for _, id := range []string{"+05:30", "UTC"} {
		z := tzMust(t, id)
		if _, ok := z.NextTransition(big.NewInt(0)); ok {
			t.Errorf("%s next", id)
		}
		if _, ok := z.PreviousTransition(big.NewInt(0)); ok {
			t.Errorf("%s previous", id)
		}
	}

	// Extremes neither overflow nor loop.
	max, min := new(big.Int).Set(MaxEpochNanoseconds), new(big.Int).Neg(MaxEpochNanoseconds)
	if _, ok := ny.NextTransition(max); ok {
		t.Error("next at max")
	}
	if _, ok := ny.PreviousTransition(min); ok {
		t.Error("previous at min")
	}
	if got, ok := ny.PreviousTransition(max); !ok || got.Cmp(max) >= 0 {
		t.Errorf("previous at max: %v %v", got, ok)
	}
	if got, ok := ny.NextTransition(min); !ok || got.Cmp(first) != 0 {
		t.Errorf("next at min: %v %v", got, ok)
	}

	// Chatham: transitions are 45-minute-offset DST changes.
	ch := tzMust(t, "Pacific/Chatham")
	got, ok := ch.NextTransition(tzNs(2024, 1, 1, 0, 0, 0))
	if !ok || ch.OffsetNanosecondsAt(got) != int64(12*3600e9+45*60e9) {
		t.Errorf("chatham: %v %v", got, ok)
	}
}

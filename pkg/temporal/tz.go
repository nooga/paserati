package temporal

import (
	"fmt"
	"math/big"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // keep zone lookup independent of the host's tz database
)

// TimeZone is a Temporal time zone: either a fixed UTC offset ("+05:30") or
// a named IANA zone. The zero value is not valid; use ParseTimeZone.
type TimeZone struct {
	id     string
	loc    *time.Location // nil for offset zones
	offset int64          // nanoseconds, offset zones only
}

var tzUTC = TimeZone{id: "UTC", loc: time.UTC}

// tzUTCAliases are the links to UTC that Temporal reports as "UTC".
var tzUTCAliases = map[string]bool{
	"utc": true, "etc/utc": true, "etc/gmt": true, "gmt": true, "etc/uct": true, "uct": true,
	"etc/universal": true, "universal": true, "etc/zulu": true, "zulu": true,
	"etc/greenwich": true, "greenwich": true, "gmt0": true, "etc/gmt0": true,
	"gmt+0": true, "gmt-0": true, "etc/gmt+0": true, "etc/gmt-0": true,
}

var (
	tzLowerOnce sync.Once
	tzLower     map[string]string
)

// ParseTimeZone resolves a time zone identifier (CanonicalizeTimeZoneName /
// ParseTimeZoneIdentifier): an offset string, or an IANA name matched
// ASCII-case-insensitively. It returns a RangeError for anything else.
func ParseTimeZone(id string) (TimeZone, error) {
	if id != "" && (id[0] == '+' || id[0] == '-') {
		off, ok := tzParseOffset(id)
		if !ok {
			return TimeZone{}, rangeErr("invalid time zone offset %q", id)
		}
		return TimeZone{id: tzFormatOffset(off), offset: off}, nil
	}
	lower := tzASCIILower(id)
	if tzUTCAliases[lower] {
		return tzUTC, nil
	}
	tzLowerOnce.Do(func() {
		tzLower = make(map[string]string, len(tzNames))
		for _, n := range tzNames {
			tzLower[tzASCIILower(n)] = n
		}
	})
	name, ok := tzLower[lower]
	if !ok {
		return TimeZone{}, rangeErr("invalid time zone %q", id)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return TimeZone{}, rangeErr("invalid time zone %q", id)
	}
	return TimeZone{id: name, loc: loc}, nil
}

func tzASCIILower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// tzParseOffset parses ±HH, ±HH:MM or ±HHMM into nanoseconds.
func tzParseOffset(s string) (int64, bool) {
	if len(s) < 3 {
		return 0, false
	}
	var digits string
	switch len(s) {
	case 3:
		digits = s[1:] + "00"
	case 5:
		digits = s[1:]
	case 6:
		if s[3] != ':' {
			return 0, false
		}
		digits = s[1:3] + s[4:]
	default:
		return 0, false
	}
	for i := 0; i < 4; i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	h := int64(digits[0]-'0')*10 + int64(digits[1]-'0')
	m := int64(digits[2]-'0')*10 + int64(digits[3]-'0')
	if h > 23 || m > 59 {
		return 0, false
	}
	ns := (h*60 + m) * 60 * 1e9
	if s[0] == '-' {
		ns = -ns
	}
	return ns, true
}

// tzFormatOffset is FormatOffsetTimeZoneIdentifier (minute precision).
func tzFormatOffset(ns int64) string {
	sign := '+'
	if ns < 0 {
		sign, ns = '-', -ns
	}
	min := ns / (60 * 1e9)
	return fmt.Sprintf("%c%02d:%02d", sign, min/60, min%60)
}

// SystemTimeZone is the host's zone (DefaultTimeZone), or UTC if it cannot
// be determined.
func SystemTimeZone() TimeZone {
	cands := []string{time.Local.String(), os.Getenv("TZ")}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if _, after, ok := strings.Cut(target, "zoneinfo/"); ok {
			cands = append(cands, after)
		}
	}
	for _, c := range cands {
		c = strings.TrimPrefix(c, ":")
		if c == "" || c == "Local" {
			continue
		}
		if tz, err := ParseTimeZone(c); err == nil {
			return tz
		}
	}
	return tzUTC
}

// ID is the identifier as Temporal reports it.
func (tz TimeZone) ID() string { return tz.id }

// IsOffset reports whether tz is a fixed-offset zone.
func (tz TimeZone) IsOffset() bool { return tz.loc == nil }

// Equal compares canonical identifiers.
func (tz TimeZone) Equal(o TimeZone) bool { return tz.id == o.id }

// tzSplit floors epoch nanoseconds into seconds and the nanosecond remainder.
func tzSplit(epochNs *big.Int) (sec, nsec int64) {
	q, r := new(big.Int).DivMod(epochNs, big.NewInt(1e9), new(big.Int))
	return q.Int64(), r.Int64()
}

func tzJoin(sec, nsec int64) *big.Int {
	n := new(big.Int).Mul(big.NewInt(sec), big.NewInt(1e9))
	return n.Add(n, big.NewInt(nsec))
}

// offsetSec is the zone's UTC offset in seconds at an epoch second.
func (tz TimeZone) offsetSec(sec int64) int64 {
	if tz.loc == nil {
		return tz.offset / 1e9
	}
	_, off := time.Unix(sec, 0).In(tz.loc).Zone()
	return int64(off)
}

// OffsetNanosecondsAt is GetOffsetNanosecondsFor.
func (tz TimeZone) OffsetNanosecondsAt(epochNs *big.Int) int64 {
	sec, _ := tzSplit(epochNs)
	return tz.offsetSec(sec) * 1e9
}

// LocalDateTime is GetISODateTimeFor: the zone's wall-clock time at epochNs.
func (tz TimeZone) LocalDateTime(epochNs *big.Int) DateTime {
	sec, nsec := tzSplit(epochNs)
	t := time.Unix(sec+tz.offsetSec(sec), 0).UTC()
	return DateTime{
		Date: Date{t.Year(), int(t.Month()), t.Day()},
		Time: Time{t.Hour(), t.Minute(), t.Second(), int(nsec / 1e6), int(nsec / 1e3 % 1e3), int(nsec % 1e3)},
	}
}

// tzLocalSec reads a local date-time as if it were UTC: epoch seconds plus
// the sub-second nanoseconds.
func tzLocalSec(dt DateTime) (sec, nsec int64) {
	t := time.Date(dt.Date.Year, time.Month(dt.Date.Month), dt.Date.Day, dt.Time.Hour, dt.Time.Minute, dt.Time.Second, 0, time.UTC)
	return t.Unix(), int64(dt.Time.Millisecond)*1e6 + int64(dt.Time.Microsecond)*1e3 + int64(dt.Time.Nanosecond)
}

// possibleSec is GetNamedTimeZoneEpochNanoseconds in seconds: the epoch
// seconds whose wall clock reads local, ascending.
func (tz TimeZone) possibleSec(local int64) []int64 {
	var out []int64
	for _, probe := range [2]int64{local - 86400, local + 86400} {
		off := tz.offsetSec(probe)
		if e := local - off; tz.offsetSec(e) == off && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	slices.Sort(out)
	return out
}

// PossibleEpochNanoseconds is GetPossibleEpochNanoseconds: no value for a
// skipped local time, two for a repeated one, ascending.
func (tz TimeZone) PossibleEpochNanoseconds(local DateTime) []*big.Int {
	sec, nsec := tzLocalSec(local)
	var out []*big.Int
	for _, e := range tz.possibleSec(sec) {
		out = append(out, tzJoin(e, nsec))
	}
	return out
}

// EpochNanosecondsFor is GetEpochNanosecondsFor with
// DisambiguatePossibleEpochNanoseconds.
func (tz TimeZone) EpochNanosecondsFor(local DateTime, d Disambiguation) (*big.Int, error) {
	sec, nsec := tzLocalSec(local)
	var e int64
	switch c := tz.possibleSec(sec); len(c) {
	case 1:
		e = c[0]
	case 2:
		switch d {
		case DisambiguateReject:
			return nil, rangeErr("local time is ambiguous in time zone %s", tz.id)
		case DisambiguateLater:
			e = c[1]
		default:
			e = c[0]
		}
	default:
		if d == DisambiguateReject {
			return nil, rangeErr("local time does not exist in time zone %s", tz.id)
		}
		// In a gap, shifting by the gap length lands on the same instant the
		// spec reaches via the earlier/later date-time.
		before, after := tz.offsetSec(sec-86400), tz.offsetSec(sec+86400)
		if d == DisambiguateEarlier {
			e = sec - after
		} else {
			e = sec - before
		}
	}
	ns := tzJoin(e, nsec)
	if new(big.Int).Abs(ns).Cmp(MaxEpochNanoseconds) > 0 {
		return nil, rangeErr("date-time is outside the representable range")
	}
	return ns, nil
}

// NextTransition is GetNamedTimeZoneNextTransition: the first offset change
// strictly after epochNs.
func (tz TimeZone) NextTransition(epochNs *big.Int) (*big.Int, bool) {
	if tz.loc == nil {
		return nil, false
	}
	sec, nsec := tzSplit(epochNs)
	t := time.Unix(sec, nsec).In(tz.loc)
	for {
		_, end := t.ZoneBounds()
		if end.IsZero() || tzOutOfRange(end.Unix()) {
			return nil, false
		}
		if tz.offsetSec(end.Unix()) != tz.offsetSec(end.Unix()-1) {
			return tzJoin(end.Unix(), 0), true
		}
		t = end
	}
}

// PreviousTransition is GetNamedTimeZonePreviousTransition: the last offset
// change strictly before epochNs.
func (tz TimeZone) PreviousTransition(epochNs *big.Int) (*big.Int, bool) {
	if tz.loc == nil {
		return nil, false
	}
	sec, nsec := tzSplit(epochNs)
	t := time.Unix(sec, nsec).In(tz.loc).Add(-1)
	for {
		start, _ := t.ZoneBounds()
		if start.IsZero() || tzOutOfRange(start.Unix()) {
			return nil, false
		}
		if tz.offsetSec(start.Unix()) != tz.offsetSec(start.Unix()-1) {
			return tzJoin(start.Unix(), 0), true
		}
		t = start.Add(-1)
	}
}

// tzOutOfRange reports whether an epoch second is outside the Instant range.
func tzOutOfRange(sec int64) bool { return sec > 8.64e12 || sec < -8.64e12 }

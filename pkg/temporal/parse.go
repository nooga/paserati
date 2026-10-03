package temporal

import (
	"strconv"
	"strings"
)

// scanner is a cursor over an ISO 8601 / RFC 9557 string.
type scanner struct {
	s string
	i int
}

func (c *scanner) eof() bool { return c.i >= len(c.s) }

func (c *scanner) peek() byte {
	if c.i < len(c.s) {
		return c.s[c.i]
	}
	return 0
}

func (c *scanner) eat(b byte) bool {
	if !c.eof() && c.s[c.i] == b {
		c.i++
		return true
	}
	return false
}

func (c *scanner) eatAny(set string) bool {
	if !c.eof() && strings.IndexByte(set, c.s[c.i]) >= 0 {
		c.i++
		return true
	}
	return false
}

func (c *scanner) isDigit(off int) bool {
	return c.i+off < len(c.s) && c.s[c.i+off] >= '0' && c.s[c.i+off] <= '9'
}

// digits reads exactly n ASCII digits.
func (c *scanner) digits(n int) (int, bool) {
	v := 0
	for k := 0; k < n; k++ {
		if !c.isDigit(k) {
			return 0, false
		}
		v = v*10 + int(c.s[c.i+k]-'0')
	}
	c.i += n
	return v, true
}

// sign reads "+" or "-". U+2212 (MINUS SIGN) is not accepted.
func (c *scanner) sign() (int, bool) {
	switch {
	case c.eat('+'):
		return 1, true
	case c.eat('-'):
		return -1, true
	}
	return 0, false
}

func (c *scanner) atSign() bool { return c.peek() == '+' || c.peek() == '-' }

// fraction reads an optional [.,]d{1,9} and returns it as nanoseconds. ok is
// false when a separator is present without 1-9 digits.
func (c *scanner) fraction() (ns int, ok bool) {
	if !c.eatAny(".,") {
		return 0, true
	}
	n := 0
	for n < 9 && c.isDigit(0) {
		ns = ns*10 + int(c.s[c.i]-'0')
		c.i++
		n++
	}
	if n == 0 || c.isDigit(0) {
		return 0, false
	}
	for ; n < 9; n++ {
		ns *= 10
	}
	return ns, true
}

func parseDaysInMonth(y, m int) int {
	switch m {
	case 2:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func parseValidMD(y, m, d int) bool {
	return m >= 1 && m <= 12 && d >= 1 && d <= parseDaysInMonth(y, m)
}

// year reads YYYY or ±YYYYYY ("-000000" is invalid).
func (c *scanner) year() (int, bool) {
	if c.atSign() {
		sg, _ := c.sign()
		v, ok := c.digits(6)
		if !ok || (sg < 0 && v == 0) {
			return 0, false
		}
		return sg * v, true
	}
	return c.digits(4)
}

// date reads YYYY-MM-DD or YYYYMMDD.
func (c *scanner) date() (Date, bool) {
	y, ok := c.year()
	if !ok {
		return Date{}, false
	}
	ext := c.eat('-')
	m, ok := c.digits(2)
	if !ok || (ext && !c.eat('-')) {
		return Date{}, false
	}
	d, ok := c.digits(2)
	if !ok || !parseValidMD(y, m, d) {
		return Date{}, false
	}
	return Date{y, m, d}, true
}

// yearMonth reads YYYY-MM or YYYYMM (DateSpecYearMonth); the day is 1.
func (c *scanner) yearMonth() (Date, bool) {
	y, ok := c.year()
	if !ok {
		return Date{}, false
	}
	c.eat('-')
	m, ok := c.digits(2)
	if !ok || m < 1 || m > 12 {
		return Date{}, false
	}
	return Date{y, m, 1}, true
}

// monthDay reads [--]MM-DD or [--]MMDD (DateSpecMonthDay); the year is 1972,
// a leap year, so that February 29 is accepted.
func (c *scanner) monthDay() (Date, bool) {
	if strings.HasPrefix(c.s[c.i:], "--") {
		c.i += 2
	}
	m, ok := c.digits(2)
	if !ok {
		return Date{}, false
	}
	c.eat('-')
	d, ok := c.digits(2)
	if !ok || !parseValidMD(1972, m, d) {
		return Date{}, false
	}
	return Date{1972, m, d}, true
}

// time reads hh, hh:mm, hh:mm:ss[.f], hhmm or hhmmss[.f]. A leap second 60
// is read as 59.
func (c *scanner) time() (Time, bool) {
	h, ok := c.digits(2)
	if !ok || h > 23 {
		return Time{}, false
	}
	var m, s, frac int
	if c.eat(':') {
		if m, ok = c.digits(2); !ok {
			return Time{}, false
		}
		if c.eat(':') {
			if s, ok = c.digits(2); !ok {
				return Time{}, false
			}
			if frac, ok = c.fraction(); !ok {
				return Time{}, false
			}
		}
	} else if c.isDigit(0) && c.isDigit(1) {
		m, _ = c.digits(2)
		if c.isDigit(0) && c.isDigit(1) {
			s, _ = c.digits(2)
			if frac, ok = c.fraction(); !ok {
				return Time{}, false
			}
		}
	}
	if m > 59 || s > 60 {
		return Time{}, false
	}
	if s == 60 {
		s = 59
	}
	return Time{h, m, s, frac / 1e6, frac / 1e3 % 1e3, frac % 1e3}, true
}

// offset reads a numeric UTC offset: ±HH, ±HH:MM, ±HHMM, ±HH:MM:SS[.f] or
// ±HHMMSS[.f].
func (c *scanner) offset() (ns int64, subMinute bool, ok bool) {
	sg, ok := c.sign()
	if !ok {
		return 0, false, false
	}
	h, ok := c.digits(2)
	if !ok || h > 23 {
		return 0, false, false
	}
	var m, s, frac int
	if c.eat(':') {
		if m, ok = c.digits(2); !ok {
			return 0, false, false
		}
		if c.eat(':') {
			subMinute = true
			if s, ok = c.digits(2); !ok {
				return 0, false, false
			}
			if frac, ok = c.fraction(); !ok {
				return 0, false, false
			}
		}
	} else if c.isDigit(0) && c.isDigit(1) {
		m, _ = c.digits(2)
		if c.isDigit(0) && c.isDigit(1) {
			subMinute = true
			s, _ = c.digits(2)
			if frac, ok = c.fraction(); !ok {
				return 0, false, false
			}
		}
	}
	if m > 59 || s > 59 {
		return 0, false, false
	}
	total := (int64(h)*3600+int64(m)*60+int64(s))*1e9 + int64(frac)
	return int64(sg) * total, subMinute, true
}

// zone reads an optional "Z" or numeric offset into p.
func (c *scanner) zone(p *Parsed) bool {
	if c.eatAny("Zz") {
		p.UTC = true
		return true
	}
	if c.atSign() {
		ns, sub, ok := c.offset()
		p.HasOffset, p.OffsetNs, p.OffsetHasSubMinute = true, ns, sub
		return ok
	}
	return true
}

func parseIsLower(b byte) bool { return b >= 'a' && b <= 'z' }
func parseIsAlpha(b byte) bool { return parseIsLower(b) || (b >= 'A' && b <= 'Z') }
func parseIsDigit(b byte) bool { return b >= '0' && b <= '9' }
func parseIsAlnum(b byte) bool { return parseIsAlpha(b) || parseIsDigit(b) }

func parseValidAnnotationKey(k string) bool {
	if k == "" || !(parseIsLower(k[0]) || k[0] == '_') {
		return false
	}
	for i := 1; i < len(k); i++ {
		if b := k[i]; !(parseIsLower(b) || parseIsDigit(b) || b == '-' || b == '_') {
			return false
		}
	}
	return true
}

// parseValidAnnotationValue: one or more non-empty alphanumeric segments
// separated by "-" (RFC 9557); calendar identifiers are checked separately.
func parseValidAnnotationValue(v string) bool {
	for _, seg := range strings.Split(v, "-") {
		if seg == "" {
			return false
		}
		for i := 0; i < len(seg); i++ {
			if !parseIsAlnum(seg[i]) {
				return false
			}
		}
	}
	return true
}

func parseIsISO8601(s string) bool {
	const want = "iso8601"
	if len(s) != len(want) {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		if b != want[i] {
			return false
		}
	}
	return true
}

// annotations reads an optional time zone annotation followed by any number
// of key=value annotations (the "Annotations" grammar of RFC 9557), applying
// the critical-flag rules of ParseISODateTime.
func (c *scanner) annotations(p *Parsed) error {
	var cals []string
	critCal := false
	for n := 0; c.peek() == '['; n++ {
		j := strings.IndexByte(c.s[c.i:], ']')
		if j < 0 {
			return rangeErr("invalid annotation in %q", c.s)
		}
		body := c.s[c.i+1 : c.i+j]
		c.i += j + 1
		crit := strings.HasPrefix(body, "!")
		body = strings.TrimPrefix(body, "!")
		k, v, isKV := strings.Cut(body, "=")
		switch {
		case isKV:
			if !parseValidAnnotationKey(k) || !parseValidAnnotationValue(v) {
				return rangeErr("invalid annotation in %q", c.s)
			}
			if k == "u-ca" {
				cals = append(cals, v)
				critCal = critCal || crit
			} else if crit {
				return rangeErr("unknown critical annotation %q", k)
			}
		case n == 0:
			if _, _, _, err := ParseTimeZoneIdentifier(body); err != nil {
				return err
			}
			p.TimeZone, p.TimeZoneCritical = body, crit
		default:
			return rangeErr("invalid annotation in %q", c.s)
		}
	}
	if len(cals) > 0 {
		// Several calendar annotations are only valid when none is critical.
		// Whether the (first) calendar is supported is up to the caller: only
		// the types that use the calendar care, and p.CalendarCritical says
		// whether an unsupported one must be an error.
		if critCal && len(cals) > 1 {
			return rangeErr("multiple calendar annotations with a critical one in %q", c.s)
		}
		p.Calendar, p.CalendarCritical = cals[0], critCal
	}
	return nil
}

func parseBad(s string) error { return rangeErr("invalid ISO 8601 string %q", s) }

// parseDateTime parses Date [T Time [Z|offset]] [tz] [annotations]; the
// goal-specific functions check which parts they require.
func parseDateTime(s string) (*Parsed, error) {
	c := &scanner{s: s}
	d, ok := c.date()
	if !ok {
		return nil, parseBad(s)
	}
	p := &Parsed{HasDate: true, Date: d}
	if c.eatAny("Tt ") {
		if p.Time, ok = c.time(); !ok {
			return nil, parseBad(s)
		}
		p.HasTime = true
		if !c.zone(p) {
			return nil, parseBad(s)
		}
	}
	if err := c.annotations(p); err != nil {
		return nil, err
	}
	if !c.eof() {
		return nil, parseBad(s)
	}
	return p, nil
}

// parseTimeOnly parses [T] Time [Z|offset] [tz] [annotations]. Without the T
// prefix, a string that is also a valid year-month or month-day is rejected
// as ambiguous.
func parseTimeOnly(s string) (*Parsed, error) {
	c := &scanner{s: s}
	hadT := c.eatAny("Tt")
	t, ok := c.time()
	if !ok {
		return nil, parseBad(s)
	}
	p := &Parsed{HasTime: true, Time: t}
	if !c.zone(p) {
		return nil, parseBad(s)
	}
	body := s[:c.i]
	if err := c.annotations(p); err != nil {
		return nil, err
	}
	if !c.eof() {
		return nil, parseBad(s)
	}
	if !hadT {
		for _, f := range []func(*scanner) (Date, bool){(*scanner).yearMonth, (*scanner).monthDay} {
			a := &scanner{s: body}
			if _, ok := f(a); ok && a.eof() {
				return nil, rangeErr("string %q is ambiguous with a date", s)
			}
		}
	}
	return p, nil
}

// parseShort parses a DateSpecYearMonth or DateSpecMonthDay followed by
// optional annotations.
func parseShort(s string, f func(*scanner) (Date, bool)) (*Parsed, error) {
	c := &scanner{s: s}
	d, ok := f(c)
	if !ok {
		return nil, parseBad(s)
	}
	p := &Parsed{HasDate: true, Date: d}
	if err := c.annotations(p); err != nil {
		return nil, err
	}
	if !c.eof() {
		return nil, parseBad(s)
	}
	return p, nil
}

func parseNoZ(p *Parsed, err error) (*Parsed, error) {
	if err != nil {
		return nil, err
	}
	if p.UTC {
		return nil, rangeErr("UTC designator is not allowed here")
	}
	return p, nil
}

// ParseInstantString parses a TemporalInstantString.
func ParseInstantString(s string) (*Parsed, error) {
	p, err := parseDateTime(s)
	if err != nil {
		return nil, err
	}
	if !p.HasTime || !(p.UTC || p.HasOffset) {
		return nil, rangeErr("instant string %q needs a time and a UTC offset", s)
	}
	return p, nil
}

// ParseZonedDateTimeString parses a TemporalZonedDateTimeString.
func ParseZonedDateTimeString(s string) (*Parsed, error) {
	p, err := parseDateTime(s)
	if err != nil {
		return nil, err
	}
	if p.TimeZone == "" {
		return nil, rangeErr("zoned date-time string %q needs a time zone annotation", s)
	}
	return p, nil
}

// ParsePlainDateTimeString parses a TemporalDateTimeString.
func ParsePlainDateTimeString(s string) (*Parsed, error) { return parseNoZ(parseDateTime(s)) }

// ParsePlainDateString parses a TemporalDateString.
func ParsePlainDateString(s string) (*Parsed, error) { return parseNoZ(parseDateTime(s)) }

// ParsePlainTimeString parses a TemporalTimeString.
func ParsePlainTimeString(s string) (*Parsed, error) {
	if p, err := parseDateTime(s); err == nil {
		if !p.HasTime {
			return nil, rangeErr("string %q has no time", s)
		}
		return parseNoZ(p, nil)
	}
	return parseNoZ(parseTimeOnly(s))
}

// ParsePlainYearMonthString parses a TemporalYearMonthString. A short form
// has day 1.
func ParsePlainYearMonthString(s string) (*Parsed, error) {
	if p, err := parseDateTime(s); err == nil {
		return parseNoZ(p, nil)
	}
	return parseShort(s, (*scanner).yearMonth)
}

// ParsePlainMonthDayString parses a TemporalMonthDayString. A short form has
// year 1972.
func ParsePlainMonthDayString(s string) (*Parsed, error) {
	if p, err := parseDateTime(s); err == nil {
		return parseNoZ(p, nil)
	}
	return parseShort(s, (*scanner).monthDay)
}

// ParseDurationString parses an ISO 8601 duration. Magnitude limits
// (IsValidDuration) are the caller's job.
func ParseDurationString(s string) (Duration, error) {
	bad := func() (Duration, error) { return Duration{}, rangeErr("invalid duration string %q", s) }
	c := &scanner{s: s}
	sg, ok := c.sign()
	if !ok {
		sg = 1
	}
	if !c.eatAny("Pp") {
		return bad()
	}
	var f [10]float64 // years .. nanoseconds
	count := 0
	readNum := func() (string, bool) {
		st := c.i
		for c.isDigit(0) {
			c.i++
		}
		return c.s[st:c.i], c.i > st
	}
	num := func(digits string) float64 {
		v, _ := strconv.ParseFloat(digits, 64) // overflow gives +Inf; the caller rejects it
		return v
	}
	designator := func(set string) int {
		if c.eof() {
			return -1
		}
		idx := strings.IndexByte(set, c.peek()&^0x20)
		if idx >= 0 {
			c.i++
		}
		return idx
	}
	// Date part: Y, M, W, D in order, integers only.
	last := -1
	for c.isDigit(0) {
		digits, _ := readNum()
		idx := designator("YMWD")
		if idx < 0 || idx <= last {
			return bad()
		}
		last = idx
		f[idx] = num(digits)
		count++
	}
	if c.eatAny("Tt") {
		last, timeCount := -1, 0
		fracSeen := false
		for !c.eof() {
			digits, ok := readNum()
			if !ok || fracSeen {
				return bad()
			}
			fracNs, hasFrac := 0, false
			if c.peek() == '.' || c.peek() == ',' {
				var fok bool
				if fracNs, fok = c.fraction(); !fok {
					return bad()
				}
				hasFrac = true
			}
			idx := designator("HMS")
			if idx < 0 || idx <= last {
				return bad()
			}
			last = idx
			f[4+idx] = num(digits)
			timeCount++
			if hasFrac {
				fracSeen = true
				// The fraction spreads into the smaller units exactly.
				rest := int64(fracNs) * [3]int64{3600, 60, 1}[idx]
				if idx == 0 {
					f[5], rest = float64(rest/60e9), rest%60e9
				}
				if idx <= 1 {
					f[6], rest = float64(rest/1e9), rest%1e9
				}
				f[7], f[8], f[9] = float64(rest/1e6), float64(rest/1e3%1e3), float64(rest%1e3)
			}
		}
		if timeCount == 0 {
			return bad()
		}
		count += timeCount
	}
	if count == 0 || !c.eof() {
		return bad()
	}
	if sg < 0 {
		for i, v := range f {
			if v != 0 {
				f[i] = -v
			}
		}
	}
	return Duration{f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9]}, nil
}

// ParseTimeZoneIdentifier splits a time zone identifier into either a
// numeric offset (minute precision) or an IANA-style name, which is not
// checked for existence.
func ParseTimeZoneIdentifier(s string) (offsetNs int64, name string, isOffset bool, err error) {
	c := &scanner{s: s}
	if c.atSign() {
		sg, _ := c.sign()
		h, ok := c.digits(2)
		m := 0
		if ok && c.eat(':') {
			m, ok = c.digits(2)
		} else if ok && c.isDigit(0) {
			m, ok = c.digits(2)
		}
		if !ok || !c.eof() || h > 23 || m > 59 {
			return 0, "", false, rangeErr("invalid time zone offset %q", s)
		}
		return int64(sg) * int64(h*60+m) * 60e9, "", true, nil
	}
	for _, comp := range strings.Split(s, "/") {
		if comp == "" || comp == "." || comp == ".." || len(comp) > 14 {
			return 0, "", false, rangeErr("invalid time zone identifier %q", s)
		}
		for i := 0; i < len(comp); i++ {
			b := comp[i]
			lead := parseIsAlpha(b) || b == '.' || b == '_'
			if !(lead || (i > 0 && (parseIsDigit(b) || b == '-' || b == '+'))) {
				return 0, "", false, rangeErr("invalid time zone identifier %q", s)
			}
		}
	}
	return 0, s, false, nil
}

// ParseDateTimeUTCOffset parses a UTCOffset with optional sub-minute
// precision, e.g. "+01:00" or "-00:00:01.5".
func ParseDateTimeUTCOffset(s string) (ns int64, hasSubMinute bool, err error) {
	c := &scanner{s: s}
	ns, sub, ok := c.offset()
	if !ok || !c.eof() {
		return 0, false, rangeErr("invalid UTC offset %q", s)
	}
	return ns, sub, nil
}

// CanonicalizeCalendarIdentifier accepts only the ISO 8601 calendar.
func CanonicalizeCalendarIdentifier(s string) (string, bool) {
	if parseIsISO8601(s) {
		return "iso8601", true
	}
	return "", false
}

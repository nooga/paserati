package builtins

import (
	"math"
	"strings"
	"time"
)

// parseDateString is the string parser behind Date.parse and new Date(string)
// (21.4.3.2). It first tries the Date Time String Format (21.4.1.32, an ISO
// 8601 subset); failing that it falls back to a tolerant parser for the
// implementation-defined formats, which must at least cover what
// Date.prototype.toString and toUTCString produce, so both round-trip.
// The constructor and Date.parse share it so they accept exactly the same
// strings. An unrecognised string gives NaN.
func parseDateString(dateStr string) float64 {
	if t, ok := parseISODate(dateStr); ok {
		return timeClip(t)
	}
	if t, ok := parseLegacyDate(dateStr); ok {
		return timeClip(t)
	}
	return math.NaN()
}

// dateMillis returns the time value of a calendar date and time, in UTC or
// in the host's local time zone. Out-of-range fields roll over like MakeDay.
func dateMillis(year, month, day, hour, min, sec, ms int, utc bool) float64 {
	loc := time.UTC
	if !utc {
		loc = time.Local
	}
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, loc)
	return float64(t.Unix())*1000 + float64(ms)
}

// isoScanner reads the fixed-width numeric fields of an ISO date string.
type isoScanner struct {
	s   string
	pos int
}

func (p *isoScanner) done() bool { return p.pos >= len(p.s) }

func (p *isoScanner) peek() byte {
	if p.done() {
		return 0
	}
	return p.s[p.pos]
}

func (p *isoScanner) accept(c byte) bool {
	if p.peek() == c {
		p.pos++
		return true
	}
	return false
}

// digits reads exactly n decimal digits.
func (p *isoScanner) digits(n int) (int, bool) {
	if p.pos+n > len(p.s) {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		c := p.s[p.pos+i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	p.pos += n
	return v, true
}

// parseISODate parses the Date Time String Format:
//
//	YYYY[-MM[-DD]][THH:mm[:ss[.sss]][Z|±HH:mm]]
//
// with ±YYYYYY expanded years. Date-only forms are UTC; date-time forms
// without an offset are local time.
func parseISODate(s string) (float64, bool) {
	p := &isoScanner{s: s}
	var year int
	switch c := p.peek(); c {
	case '+', '-':
		p.pos++
		y, ok := p.digits(6)
		if !ok || (c == '-' && y == 0) { // -000000 is not a valid year
			return 0, false
		}
		year = y
		if c == '-' {
			year = -y
		}
	default:
		y, ok := p.digits(4)
		if !ok {
			return 0, false
		}
		year = y
	}
	month, day := 1, 1
	if p.accept('-') {
		m, ok := p.digits(2)
		if !ok || m < 1 || m > 12 {
			return 0, false
		}
		month = m
		if p.accept('-') {
			d, ok := p.digits(2)
			if !ok || d < 1 || d > daysInMonth(year, month) {
				return 0, false
			}
			day = d
		}
	}
	if p.done() {
		return dateMillis(year, month, day, 0, 0, 0, 0, true), true
	}
	if !p.accept('T') && !p.accept('t') {
		return 0, false
	}
	hour, ok := p.digits(2)
	if !ok || !p.accept(':') {
		return 0, false
	}
	min, ok := p.digits(2)
	if !ok {
		return 0, false
	}
	sec, ms := 0, 0
	if p.accept(':') {
		if sec, ok = p.digits(2); !ok {
			return 0, false
		}
		if p.accept('.') {
			// At least one fraction digit; milliseconds are the first three.
			start := p.pos
			for !p.done() && p.peek() >= '0' && p.peek() <= '9' {
				p.pos++
			}
			frac := p.s[start:p.pos]
			if frac == "" {
				return 0, false
			}
			for len(frac) < 3 {
				frac += "0"
			}
			ms = int(frac[0]-'0')*100 + int(frac[1]-'0')*10 + int(frac[2]-'0')
		}
	}
	if hour > 24 || min > 59 || sec > 59 || (hour == 24 && (min != 0 || sec != 0 || ms != 0)) {
		return 0, false
	}
	if p.done() {
		return dateMillis(year, month, day, hour, min, sec, ms, false), true
	}
	offset := 0
	switch c := p.peek(); c {
	case 'Z', 'z':
		p.pos++
	case '+', '-':
		p.pos++
		oh, ok := p.digits(2)
		if !ok || !p.accept(':') {
			return 0, false
		}
		om, ok := p.digits(2)
		if !ok || oh > 23 || om > 59 {
			return 0, false
		}
		offset = oh*60 + om
		if c == '-' {
			offset = -offset
		}
	default:
		return 0, false
	}
	if !p.done() {
		return 0, false
	}
	return dateMillis(year, month, day, hour, min, sec, ms, true) - float64(offset)*60000, true
}

// looksISO reports whether s starts like the Date Time String Format: a
// signed year, or four digits followed by '-'.
func looksISO(s string) bool {
	if s != "" && (s[0] == '+' || s[0] == '-') {
		return true
	}
	return len(s) > 4 && isDigit(s[0]) && isDigit(s[1]) && isDigit(s[2]) && isDigit(s[3]) && s[4] == '-'
}

func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

var legacyMonths = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
var legacyWeekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// parseLegacyDate is the fallback for implementation-defined formats. It
// reads words (month and weekday names, AM/PM, UTC/GMT/Z), a time of day
// (H:mm[:ss[.sss]]), a numeric offset (±hhmm or ±hh:mm, optionally after
// GMT/UTC) and up to three date numbers, ignoring parenthesised comments.
// Covers toString() ("Tue Jan 01 2019 00:00:00 GMT+0100 (CET)"),
// toUTCString()/RFC 1123 ("Tue, 01 Jan 2019 00:00:00 GMT"), "Jan 1, 2019",
// "1 January 2019 10:00 PM", "2019/01/02" and "01/02/2019" (month first).
// Without an offset the result is local time.
func parseLegacyDate(s string) (float64, bool) {
	if looksISO(s) {
		return 0, false // an invalid ISO string is invalid, not legacy
	}
	month := -1
	var nums []int
	var numLens []int
	hour, min, sec, ms := -1, 0, 0, 0
	ampm := ""
	hasOffset := false
	offset := 0

	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '(':
			depth := 0
			for i < len(s) {
				if s[i] == '(' {
					depth++
				} else if s[i] == ')' {
					depth--
					if depth == 0 {
						i++
						break
					}
				}
				i++
			}
			if depth != 0 {
				return 0, false
			}
		case isASCIILetter(c):
			start := i
			for i < len(s) && isASCIILetter(s[i]) {
				i++
			}
			word := strings.ToLower(s[start:i])
			switch {
			case word == "am" || word == "pm":
				if ampm != "" {
					return 0, false
				}
				ampm = word
			case word == "utc" || word == "gmt" || word == "ut" || word == "z":
				hasOffset = true
			case word == "t" && hour < 0:
				// ISO-like "T" between date and time
			case len(word) >= 3 && matchesPrefix(legacyMonths, word):
				if month >= 0 {
					return 0, false
				}
				month = indexPrefix(legacyMonths, word) + 1
			case len(word) >= 3 && matchesPrefix(legacyWeekdays, word):
				// weekday names carry no information
			default:
				return 0, false
			}
		case (c == '+' || c == '-') && hour >= 0 && i+1 < len(s) && isDigit(s[i+1]):
			// Offset after the time: +hhmm, +hh:mm or +hh.
			sign := 1
			if c == '-' {
				sign = -1
			}
			i++
			start := i
			for i < len(s) && isDigit(s[i]) {
				i++
			}
			d := s[start:i]
			var oh, om int
			switch {
			case len(d) == 4:
				oh, om = atoiASCII(d[:2]), atoiASCII(d[2:])
			case len(d) <= 2 && i < len(s) && s[i] == ':':
				oh = atoiASCII(d)
				i++
				start = i
				for i < len(s) && isDigit(s[i]) {
					i++
				}
				if i-start != 2 {
					return 0, false
				}
				om = atoiASCII(s[start:i])
			case len(d) <= 2:
				oh = atoiASCII(d)
			default:
				return 0, false
			}
			if oh > 23 || om > 59 {
				return 0, false
			}
			hasOffset = true
			offset = sign * (oh*60 + om)
		case isDigit(c):
			start := i
			for i < len(s) && isDigit(s[i]) {
				i++
			}
			n := atoiASCII(s[start:i])
			if i < len(s) && s[i] == ':' && hour < 0 {
				// Time of day: H:mm[:ss[.sss]]
				hour = n
				i++
				var ok bool
				if min, i, ok = readFixed(s, i, 2); !ok {
					return 0, false
				}
				if i < len(s) && s[i] == ':' {
					if sec, i, ok = readFixed(s, i+1, 2); !ok {
						return 0, false
					}
					if i < len(s) && s[i] == '.' {
						start := i + 1
						i = start
						for i < len(s) && isDigit(s[i]) {
							i++
						}
						frac := s[start:i]
						if frac == "" {
							return 0, false
						}
						for len(frac) < 3 {
							frac += "0"
						}
						ms = atoiASCII(frac[:3])
					}
				}
				continue
			}
			if len(nums) == 3 {
				return 0, false
			}
			nums = append(nums, n)
			numLens = append(numLens, i-start)
		case c == ' ' || c == ',' || c == '/' || c == '-' || c == '.' || c == '\t':
			i++
		default:
			return 0, false
		}
	}

	// Assign the date numbers.
	var year, mon, day int
	switch {
	case month > 0 && len(nums) == 2:
		// "Jan 1 2019", "1 Jan 2019", "2019 Jan 1"
		mon = month
		if numLens[0] >= 3 || nums[0] > 31 {
			year, day = expandYear(nums[0], numLens[0]), nums[1]
		} else {
			day, year = nums[0], expandYear(nums[1], numLens[1])
		}
	case month > 0 && len(nums) == 1:
		// "Jan 2019" or "1 Jan": without a year it isn't a date
		if numLens[0] < 3 && nums[0] <= 31 {
			return 0, false
		}
		mon, day, year = month, 1, nums[0]
	case month < 0 && len(nums) == 3:
		if numLens[0] >= 3 || nums[0] > 31 {
			year, mon, day = nums[0], nums[1], nums[2] // Y/M/D
		} else {
			mon, day, year = nums[0], nums[1], expandYear(nums[2], numLens[2]) // M/D/Y
		}
	default:
		return 0, false
	}
	if mon < 1 || mon > 12 || day < 1 || day > 31 {
		return 0, false
	}

	if hour < 0 {
		hour = 0
		if ampm != "" {
			return 0, false
		}
	}
	switch ampm {
	case "am":
		if hour > 12 {
			return 0, false
		}
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour > 12 {
			return 0, false
		}
		if hour < 12 {
			hour += 12
		}
	}
	if hour > 24 || min > 59 || sec > 59 {
		return 0, false
	}

	if hasOffset {
		return dateMillis(year, mon, day, hour, min, sec, ms, true) - float64(offset)*60000, true
	}
	return dateMillis(year, mon, day, hour, min, sec, ms, false), true
}

// expandYear maps a two-digit year to 1950-2049, as browsers do.
func expandYear(y, digits int) int {
	if digits > 2 {
		return y
	}
	if y < 50 {
		return 2000 + y
	}
	return 1900 + y
}

func readFixed(s string, i, n int) (int, int, bool) {
	if i+n > len(s) {
		return 0, i, false
	}
	for j := i; j < i+n; j++ {
		if !isDigit(s[j]) {
			return 0, i, false
		}
	}
	return atoiASCII(s[i : i+n]), i + n, true
}

func matchesPrefix(names []string, word string) bool { return indexPrefix(names, word) >= 0 }

// indexPrefix finds the name word starts with (its first three letters).
func indexPrefix(names []string, word string) int {
	for i, n := range names {
		if strings.HasPrefix(word, n) {
			return i
		}
	}
	return -1
}

func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func atoiASCII(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

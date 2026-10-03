package temporal

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// FormatYear is PadISOYear: four digits for 0..9999, else a sign and six.
func FormatYear(y int) string {
	if y >= 0 && y <= 9999 {
		return fmt.Sprintf("%04d", y)
	}
	if y < 0 {
		return fmt.Sprintf("-%06d", -y)
	}
	return fmt.Sprintf("+%06d", y)
}

// FormatDate renders YYYY-MM-DD.
func FormatDate(d Date) string {
	return fmt.Sprintf("%s-%02d-%02d", FormatYear(d.Year), d.Month, d.Day)
}

// fmtFraction renders the first n digits of a nine-digit fraction (n = -1:
// all but trailing zeros) including the leading point; "" if none.
func fmtFraction(frac int, n int) string {
	digits := fmt.Sprintf("%09d", frac)
	if n < 0 {
		digits = strings.TrimRight(digits, "0")
	} else {
		digits = digits[:n]
	}
	if digits == "" {
		return ""
	}
	return "." + digits
}

// FormatTime is FormatTimeString. precision: -1 auto, -2 minute, 0..9
// fraction digits.
func FormatTime(t Time, precision int) string {
	hm := fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
	if precision == -2 {
		return hm
	}
	frac := t.Millisecond*1e6 + t.Microsecond*1e3 + t.Nanosecond
	return fmt.Sprintf("%s:%02d%s", hm, t.Second, fmtFraction(frac, precision))
}

// FormatDateTime renders the date, "T" and the time.
func FormatDateTime(dt DateTime, precision int) string {
	return FormatDate(dt.Date) + "T" + FormatTime(dt.Time, precision)
}

// FormatOffset is FormatDateTimeUTCOffsetRounded when roundToMinute (nearest
// minute, ties away from zero) and the full-precision form otherwise.
func FormatOffset(ns int64, roundToMinute bool) string {
	sign := "+"
	if ns < 0 {
		sign, ns = "-", -ns
	}
	if roundToMinute {
		ns = (ns + 30e9) / 60e9 * 60e9
	}
	h, m, s, frac := ns/3600e9, ns/60e9%60, ns/1e9%60, int(ns%1e9)
	out := fmt.Sprintf("%s%02d:%02d", sign, h, m)
	if s != 0 || frac != 0 {
		out += fmt.Sprintf(":%02d%s", s, fmtFraction(frac, -1))
	}
	return out
}

func fmtInt(v float64) string { return strconv.FormatFloat(math.Abs(v), 'f', 0, 64) }

// FormatDuration is TemporalDurationToString. precision is -1 (auto) or the
// number of fractional second digits (truncating; rounding is the caller's).
func FormatDuration(d Duration, precision int) string {
	neg := false
	for _, v := range [...]float64{d.Years, d.Months, d.Weeks, d.Days, d.Hours, d.Minutes, d.Seconds, d.Milliseconds, d.Microseconds, d.Nanoseconds} {
		if v < 0 {
			neg = true
		}
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteByte('P')
	for _, c := range [...]struct {
		v float64
		u byte
	}{{d.Years, 'Y'}, {d.Months, 'M'}, {d.Weeks, 'W'}, {d.Days, 'D'}} {
		if c.v != 0 {
			b.WriteString(fmtInt(c.v))
			b.WriteByte(c.u)
		}
	}

	total := new(big.Int)
	for _, p := range [...]struct {
		v float64
		m int64
	}{{d.Seconds, 1e9}, {d.Milliseconds, 1e6}, {d.Microseconds, 1e3}, {d.Nanoseconds, 1}} {
		f, _ := new(big.Float).SetFloat64(math.Abs(p.v)).Int(nil)
		total.Add(total, f.Mul(f, big.NewInt(p.m)))
	}
	secs, sub := new(big.Int).QuoRem(total, big.NewInt(1e9), new(big.Int))

	var tp strings.Builder
	if d.Hours != 0 {
		tp.WriteString(fmtInt(d.Hours) + "H")
	}
	if d.Minutes != 0 {
		tp.WriteString(fmtInt(d.Minutes) + "M")
	}
	zeroMinutesAndHigher := d.Years == 0 && d.Months == 0 && d.Weeks == 0 && d.Days == 0 && d.Hours == 0 && d.Minutes == 0
	if total.Sign() != 0 || zeroMinutesAndHigher || precision >= 0 {
		tp.WriteString(secs.String() + fmtFraction(int(sub.Int64()), precision) + "S")
	}
	if tp.Len() > 0 {
		b.WriteByte('T')
		b.WriteString(tp.String())
	}
	return b.String()
}

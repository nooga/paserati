package builtins

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nooga/paserati/pkg/vm"
)

// intlDateTimeFormat holds an Intl.DateTimeFormat's resolved slots. Only
// English locale data is available (other locales resolve to the default
// en-US, which resolvedOptions reports), with regional field order and
// hour cycle.
type intlDateTimeFormat struct {
	locale, calendar, numberingSystem string
	timeZone                          string
	loc                               *time.Location
	hourCycle                         string // "" when there is no hour
	weekday, era, year, month, day    string
	dayPeriod, hour, minute, second   string
	fsd                               int
	timeZoneName                      string
	dateStyle, timeStyle              string
	region                            string
	formatBound                       bool
	boundFormat                       vm.Value
}

const (
	intlNNBSP     = "\u202f" // ICU 72+: between the time and AM/PM
	intlThinSpace = "\u2009" // around a range dash
)

// ---- Locale availability ----

func intlIsEnglishLocale(locale string) bool {
	return locale == "en" || strings.HasPrefix(locale, "en-")
}

// intlUSStyleRegions write numeric dates month-first; intlH12Regions use a
// 12-hour clock.
var intlUSStyleRegions = map[string]bool{"": true, "US": true, "PH": true, "PR": true, "UM": true, "VI": true, "AS": true, "GU": true, "MP": true}
var intlH12Regions = map[string]bool{
	"": true, "US": true, "CA": true, "AU": true, "NZ": true, "IN": true, "PH": true, "PK": true, "EG": true,
	"SA": true, "MY": true, "PR": true, "UM": true, "VI": true, "AS": true, "GU": true, "MP": true, "JM": true,
	"BD": true, "SG": false,
}

// intlResolveLocaleKeys is ResolveLocale over several relevant extension
// keys, for constructors whose available locales are those accepted by
// available (falling back to en-US). options holds each key's option value
// ("" when absent, intlNullOption when an option such as hour12 voids the
// key); supported decides whether a value can be used.
const intlNullOption = "\x00null"

func intlResolveLocaleKeys(requested []string, available func(string) bool, keys []string, options map[string]string, supported map[string]func(string) bool) (string, map[string]string) {
	found, foundRequested := "", ""
	for _, r := range requested {
		candidate, ok := intlBestAvailableLocale(intlRemoveUnicodeExtensions(r))
		for ok && !available(candidate) {
			pos := strings.LastIndexByte(candidate, '-')
			if pos < 0 {
				ok = false
				break
			}
			candidate = candidate[:pos]
		}
		if ok {
			found, foundRequested = candidate, r
			break
		}
	}
	if found == "" {
		found = intlDefaultLocale()
		if !available(found) {
			found = "en-US"
		}
	}
	values := map[string]string{}
	var ext []string
	for _, key := range keys {
		value, fromExt := "", ""
		if foundRequested != "" {
			if v, ok := intlUnicodeExtensionValue(foundRequested, key); ok && v != "" && supported[key](v) {
				value, fromExt = v, v
			}
		}
		if opt := options[key]; opt == intlNullOption {
			value, fromExt = "", ""
		} else if opt != "" && supported[key](opt) && opt != value {
			value, fromExt = opt, ""
		}
		if fromExt != "" {
			ext = append(ext, key, fromExt)
		}
		values[key] = value
	}
	if len(ext) > 0 {
		found += "-u-" + strings.Join(ext, "-")
	}
	return found, values
}

// ---- Time zones ----

var (
	intlDefaultTZOnce  sync.Once
	intlDefaultTZValue string
)

// intlDefaultTimeZone is DefaultTimeZone: $TZ, else the /etc/localtime
// link target, else UTC.
func intlDefaultTimeZone() string {
	intlDefaultTZOnce.Do(func() {
		intlDefaultTZValue = "UTC"
		if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
			if name, _, ok := intlCanonicalTimeZone(tz); ok {
				intlDefaultTZValue = name
				return
			}
		}
		if target, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
			if i := strings.Index(target, "zoneinfo/"); i >= 0 {
				if name, _, ok := intlCanonicalTimeZone(target[i+len("zoneinfo/"):]); ok {
					intlDefaultTZValue = name
				}
			}
		}
	})
	return intlDefaultTZValue
}

// intlCanonicalTimeZone validates an IANA name (case-insensitively) or a
// UTC offset ("+01:00", "-0530") and returns its canonical form.
func intlCanonicalTimeZone(name string) (string, *time.Location, bool) {
	if off, ok := intlParseOffsetTimeZone(name); ok {
		sign := "+"
		if off < 0 {
			sign, off = "-", -off
		}
		canonical := fmt.Sprintf("%s%02d:%02d", sign, off/60, off%60)
		return canonical, time.FixedZone(canonical, intlOffsetSeconds(canonical)), true
	}
	// UTC and its aliases keep the spelling asked for (case-normalized):
	// time zone identifiers are no longer canonicalized to their primary.
	for _, alias := range intlUTCAliases {
		if strings.EqualFold(name, alias) {
			return alias, time.UTC, true
		}
	}
	for i := 0; i < len(name); i++ {
		if name[i] >= 0x80 {
			return "", nil, false
		}
	}
	if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		return "", nil, false
	}
	for _, candidate := range []string{name, intlTitleZone(name)} {
		if loc, err := time.LoadLocation(candidate); err == nil && candidate != "Local" && candidate != "" {
			return candidate, loc, true
		}
	}
	return "", nil, false
}

// intlTitleZone recases "america/new_york" as "America/New_York".
func intlTitleZone(name string) string {
	segments := strings.Split(name, "/")
	for i, seg := range segments {
		var b strings.Builder
		upperNext := true
		for _, r := range strings.ToLower(seg) {
			if upperNext && r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			upperNext = r == '_' || r == '-'
			b.WriteRune(r)
		}
		segments[i] = b.String()
	}
	if len(segments) == 2 && strings.EqualFold(segments[0], "etc") {
		segments[1] = strings.ToUpper(segments[1])
	}
	return strings.Join(segments, "/")
}

var intlUTCAliases = []string{
	"UTC", "Etc/UTC", "Etc/GMT", "GMT", "Etc/UCT", "UCT", "Etc/Universal", "Universal", "Etc/Zulu", "Zulu",
	"Etc/Greenwich", "Greenwich", "Etc/GMT0", "GMT0", "Etc/GMT+0", "GMT+0", "Etc/GMT-0", "GMT-0",
}

func intlIsUTCAlias(name string) bool {
	for _, alias := range intlUTCAliases {
		if alias == name {
			return true
		}
	}
	return false
}

func intlParseOffsetTimeZone(s string) (int, bool) {
	if len(s) < 3 || (s[0] != '+' && s[0] != '-') {
		return 0, false
	}
	body := strings.Replace(s[1:], ":", "", 1)
	if len(body) != 2 && len(body) != 4 || !intlAll(body, intlIsDigit) {
		return 0, false
	}
	if len(s[1:]) == 5 && s[3] != ':' {
		return 0, false
	}
	h, _ := strconv.Atoi(body[:2])
	m := 0
	if len(body) == 4 {
		m, _ = strconv.Atoi(body[2:])
	}
	if h > 23 || m > 59 {
		return 0, false
	}
	off := h*60 + m
	if s[0] == '-' {
		off = -off
	}
	return off, true
}

func intlOffsetSeconds(canonical string) int {
	off, _ := intlParseOffsetTimeZone(canonical)
	return off * 60
}

// ---- CreateDateTimeFormat ----

var intlDTFComponents = []struct {
	name   string
	values []string
}{
	{"weekday", []string{"narrow", "short", "long"}},
	{"era", []string{"narrow", "short", "long"}},
	{"year", []string{"2-digit", "numeric"}},
	{"month", []string{"2-digit", "numeric", "narrow", "short", "long"}},
	{"day", []string{"2-digit", "numeric"}},
	{"dayPeriod", []string{"narrow", "short", "long"}},
	{"hour", []string{"2-digit", "numeric"}},
	{"minute", []string{"2-digit", "numeric"}},
	{"second", []string{"2-digit", "numeric"}},
	{"fractionalSecondDigits", nil},
	{"timeZoneName", []string{"short", "long", "shortOffset", "longOffset", "shortGeneric", "longGeneric"}},
}

func (dtf *intlDateTimeFormat) field(name string) *string {
	switch name {
	case "weekday":
		return &dtf.weekday
	case "era":
		return &dtf.era
	case "year":
		return &dtf.year
	case "month":
		return &dtf.month
	case "day":
		return &dtf.day
	case "dayPeriod":
		return &dtf.dayPeriod
	case "hour":
		return &dtf.hour
	case "minute":
		return &dtf.minute
	case "second":
		return &dtf.second
	case "timeZoneName":
		return &dtf.timeZoneName
	}
	return nil
}

// intlNewDateTimeFormat is CreateDateTimeFormat; required is "date",
// "time" or "any" and defaults "date", "time" or "all".
func intlNewDateTimeFormat(vmInstance *vm.VM, locales, optionsArg vm.Value, required, defaults string) (*intlDateTimeFormat, error) {
	const owner = "Intl.DateTimeFormat"
	requested, err := intlCanonicalizeLocaleList(vmInstance, locales)
	if err != nil {
		return nil, err
	}
	options, err := intlCoerceOptionsToObject(vmInstance, optionsArg)
	if err != nil {
		return nil, err
	}
	if _, err := intlGetStringOption(vmInstance, options, "localeMatcher", owner, intlLocaleMatchers, "best fit"); err != nil {
		return nil, err
	}
	calendar, err := intlGetStringOption(vmInstance, options, "calendar", owner, nil, "\x00")
	if err != nil {
		return nil, err
	}
	if calendar == "\x00" {
		calendar = ""
	} else if !intlIsUnicodeTypeSequence(calendar) {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid calendar : %s", calendar))
	}
	nu, err := intlGetStringOption(vmInstance, options, "numberingSystem", owner, nil, "\x00")
	if err != nil {
		return nil, err
	}
	if nu == "\x00" {
		nu = ""
	} else if !intlIsUnicodeTypeSequence(nu) {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid numberingSystem : %s", nu))
	}
	hour12Val, err := vmInstance.GetProperty(options, "hour12")
	if err != nil {
		return nil, err
	}
	hour12Set, hour12 := hour12Val.Type() != vm.TypeUndefined, hour12Val.IsTruthy()
	hourCycle, err := intlGetStringOption(vmInstance, options, "hourCycle", owner, []string{"h11", "h12", "h23", "h24"}, "")
	if err != nil {
		return nil, err
	}
	if hour12Set {
		hourCycle = intlNullOption
	}

	dtf := &intlDateTimeFormat{}
	isHC := func(s string) bool { return s == "h11" || s == "h12" || s == "h23" || s == "h24" }
	locale, values := intlResolveLocaleKeys(requested, intlIsEnglishLocale, []string{"ca", "hc", "nu"},
		map[string]string{"ca": strings.ToLower(calendar), "hc": hourCycle, "nu": strings.ToLower(nu)},
		map[string]func(string) bool{
			"ca": func(s string) bool { return s == "gregory" },
			"hc": isHC,
			"nu": intlIsSupportedNumberingSystem,
		})
	dtf.locale = locale
	dtf.calendar = "gregory"
	dtf.numberingSystem = values["nu"]
	if dtf.numberingSystem == "" {
		dtf.numberingSystem = "latn"
	}
	_, region := intlLanguageRegion(intlRemoveUnicodeExtensions(locale))
	dtf.region = region

	tzVal, err := vmInstance.GetProperty(options, "timeZone")
	if err != nil {
		return nil, err
	}
	if tzVal.Type() == vm.TypeUndefined {
		dtf.timeZone = intlDefaultTimeZone()
	} else {
		tz, err := getStringValueWithVM(vmInstance, tzVal)
		if err != nil {
			return nil, err
		}
		dtf.timeZone = tz
	}
	name, loc, ok := intlCanonicalTimeZone(dtf.timeZone)
	if !ok {
		return nil, vmInstance.NewRangeError(fmt.Sprintf("Invalid time zone specified: %s", dtf.timeZone))
	}
	dtf.timeZone, dtf.loc = name, loc

	explicit := false
	for _, c := range intlDTFComponents {
		if c.values == nil {
			fsd, set, err := intlGetNumberOption(vmInstance, options, "fractionalSecondDigits", 1, 3, 0)
			if err != nil {
				return nil, err
			}
			if set {
				dtf.fsd = fsd
				explicit = true
			}
			continue
		}
		v, err := intlGetStringOption(vmInstance, options, c.name, owner, c.values, "")
		if err != nil {
			return nil, err
		}
		if v != "" {
			*dtf.field(c.name) = v
			explicit = true
		}
	}
	if _, err := intlGetStringOption(vmInstance, options, "formatMatcher", owner, []string{"basic", "best fit"}, "best fit"); err != nil {
		return nil, err
	}
	styles := []string{"full", "long", "medium", "short"}
	if dtf.dateStyle, err = intlGetStringOption(vmInstance, options, "dateStyle", owner, styles, ""); err != nil {
		return nil, err
	}
	if dtf.timeStyle, err = intlGetStringOption(vmInstance, options, "timeStyle", owner, styles, ""); err != nil {
		return nil, err
	}
	if dtf.dateStyle != "" || dtf.timeStyle != "" {
		if explicit {
			return nil, vmInstance.NewTypeError("Can't set option " + intlFirstExplicit(dtf) + " when dateStyle or timeStyle is used")
		}
		if required == "date" && dtf.timeStyle != "" {
			return nil, vmInstance.NewTypeError("Invalid option : timeStyle")
		}
		if required == "time" && dtf.dateStyle != "" {
			return nil, vmInstance.NewTypeError("Invalid option : dateStyle")
		}
	} else {
		needDefaults := true
		if required == "date" || required == "any" {
			needDefaults = needDefaults && dtf.weekday == "" && dtf.year == "" && dtf.month == "" && dtf.day == ""
		}
		if required == "time" || required == "any" {
			needDefaults = needDefaults && dtf.dayPeriod == "" && dtf.hour == "" && dtf.minute == "" && dtf.second == "" && dtf.fsd == 0
		}
		if needDefaults && (defaults == "date" || defaults == "all") {
			dtf.year, dtf.month, dtf.day = "numeric", "numeric", "numeric"
		}
		if needDefaults && (defaults == "time" || defaults == "all") {
			dtf.hour, dtf.minute, dtf.second = "numeric", "numeric", "numeric"
		}
	}

	if dtf.hour != "" || dtf.timeStyle != "" {
		hcDefault := "h23"
		if intlH12Regions[region] {
			hcDefault = "h12"
		}
		hc := values["hc"]
		if hc == "" {
			hc = hcDefault
		}
		if hour12Set {
			if hour12 {
				hc = "h12"
				if hcDefault == "h23" && region == "JP" {
					hc = "h11"
				}
			} else {
				hc = "h23"
			}
		}
		dtf.hourCycle = hc
	}
	return dtf, nil
}

func intlFirstExplicit(dtf *intlDateTimeFormat) string {
	for _, c := range intlDTFComponents {
		if c.values == nil {
			if dtf.fsd != 0 {
				return c.name
			}
			continue
		}
		if *dtf.field(c.name) != "" {
			return c.name
		}
	}
	return ""
}

func intlLanguageRegion(locale string) (string, string) {
	parts := strings.Split(locale, "-")
	lang := parts[0]
	for _, p := range parts[1:] {
		if intlIsRegionSubtag(p) {
			return lang, strings.ToUpper(p)
		}
	}
	return lang, ""
}

// ---- Formatting ----

var intlMonthNames = [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
var intlWeekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// intlTimeClip is TimeClip plus the RangeError format() raises for NaN.
func intlTimeClip(vmInstance *vm.VM, x float64) (float64, error) {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 8.64e15 {
		return 0, vmInstance.NewRangeError("Invalid time value")
	}
	return math.Trunc(x) + 0, nil
}

func (dtf *intlDateTimeFormat) digits(s string) string {
	return intlTransliterate(s, dtf.numberingSystem)
}

func twoDigit(n int) string {
	return fmt.Sprintf("%02d", n%100)
}

// partsFor formats epoch milliseconds.
func (dtf *intlDateTimeFormat) partsFor(ms float64) []intlPart {
	t := time.UnixMilli(int64(ms)).In(dtf.loc)
	if dtf.dateStyle != "" || dtf.timeStyle != "" {
		return dtf.styleParts(t)
	}
	date := dtf.dateParts(t, dtf.weekday, dtf.era, dtf.year, dtf.month, dtf.day)
	tm := dtf.timeParts(t, dtf.hour, dtf.minute, dtf.second, dtf.fsd, dtf.dayPeriod, dtf.timeZoneName)
	switch {
	case len(date) == 0:
		return tm
	case len(tm) == 0:
		return date
	}
	return append(append(date, intlPart{"literal", ", "}), tm...)
}

func (dtf *intlDateTimeFormat) styleParts(t time.Time) []intlPart {
	usStyle := intlUSStyleRegions[dtf.region]
	var date []intlPart
	switch dtf.dateStyle {
	case "full":
		date = dtf.dateParts(t, "long", "", "numeric", "long", "numeric")
		if !usStyle && len(date) > 1 && date[1].value == ", " {
			date[1].value = " "
		}
	case "long":
		date = dtf.dateParts(t, "", "", "numeric", "long", "numeric")
	case "medium":
		date = dtf.dateParts(t, "", "", "numeric", "short", "numeric")
	case "short":
		if usStyle {
			date = dtf.dateParts(t, "", "", "2-digit", "numeric", "numeric")
		} else {
			date = dtf.dateParts(t, "", "", "numeric", "2-digit", "2-digit")
		}
	}
	var tm []intlPart
	switch dtf.timeStyle {
	case "full":
		tm = dtf.timeParts(t, "numeric", "2-digit", "2-digit", 0, "", "long")
	case "long":
		tm = dtf.timeParts(t, "numeric", "2-digit", "2-digit", 0, "", "short")
	case "medium":
		tm = dtf.timeParts(t, "numeric", "2-digit", "2-digit", 0, "", "")
	case "short":
		tm = dtf.timeParts(t, "numeric", "2-digit", "", 0, "", "")
	}
	switch {
	case len(date) == 0:
		return tm
	case len(tm) == 0:
		return date
	}
	sep := ", "
	if dtf.dateStyle == "full" || dtf.dateStyle == "long" {
		sep = " at "
	}
	return append(append(date, intlPart{"literal", sep}), tm...)
}

func (dtf *intlDateTimeFormat) dateParts(t time.Time, weekday, era, year, month, day string) []intlPart {
	var yearPart, monthPart, dayPart, weekdayPart, eraPart *intlPart
	y := t.Year()
	if year != "" {
		display := y
		if y <= 0 {
			display = 1 - y
			if era == "" {
				era = "short"
			}
		}
		s := strconv.Itoa(display)
		if year == "2-digit" {
			s = twoDigit(display)
		}
		yearPart = &intlPart{"year", dtf.digits(s)}
	}
	if month != "" {
		m := int(t.Month())
		var s string
		switch month {
		case "numeric":
			s = dtf.digits(strconv.Itoa(m))
		case "2-digit":
			s = dtf.digits(twoDigit(m))
		case "long":
			s = intlMonthNames[m-1]
		case "short":
			s = intlMonthNames[m-1][:3]
		case "narrow":
			s = intlMonthNames[m-1][:1]
		}
		monthPart = &intlPart{"month", s}
	}
	if day != "" {
		s := strconv.Itoa(t.Day())
		if day == "2-digit" {
			s = twoDigit(t.Day())
		}
		dayPart = &intlPart{"day", dtf.digits(s)}
	}
	if weekday != "" {
		name := intlWeekdayNames[t.Weekday()]
		switch weekday {
		case "short":
			name = name[:3]
		case "narrow":
			name = name[:1]
		}
		weekdayPart = &intlPart{"weekday", name}
	}
	if era != "" {
		ad := y > 0
		var s string
		switch era {
		case "long":
			s = map[bool]string{true: "Anno Domini", false: "Before Christ"}[ad]
		case "narrow":
			s = map[bool]string{true: "A", false: "B"}[ad]
		default:
			s = map[bool]string{true: "AD", false: "BC"}[ad]
		}
		eraPart = &intlPart{"era", s}
	}

	lit := func(s string) intlPart { return intlPart{"literal", s} }
	var out []intlPart
	textual := monthPart != nil && (month == "long" || month == "short" || month == "narrow")
	usStyle := intlUSStyleRegions[dtf.region]
	switch {
	case textual:
		if usStyle {
			out = append(out, *monthPart)
			if dayPart != nil {
				out = append(out, lit(" "), *dayPart)
			}
			if yearPart != nil {
				if dayPart != nil {
					out = append(out, lit(", "))
				} else {
					out = append(out, lit(" "))
				}
				out = append(out, *yearPart)
			}
		} else {
			if dayPart != nil {
				out = append(out, *dayPart, lit(" "))
			}
			out = append(out, *monthPart)
			if yearPart != nil {
				out = append(out, lit(" "), *yearPart)
			}
		}
	case monthPart != nil || (dayPart != nil && yearPart != nil):
		var order []*intlPart
		sep := "/"
		switch {
		case dtf.region == "CA":
			order, sep = []*intlPart{yearPart, monthPart, dayPart}, "-"
			if monthPart != nil && month == "numeric" {
				monthPart.value = dtf.digits(twoDigit(int(t.Month())))
			}
			if dayPart != nil && day == "numeric" {
				dayPart.value = dtf.digits(twoDigit(t.Day()))
			}
		case usStyle:
			order = []*intlPart{monthPart, dayPart, yearPart}
		default:
			order = []*intlPart{dayPart, monthPart, yearPart}
			// en-GB style pads the numeric day and month.
			if monthPart != nil && month == "numeric" && dayPart != nil {
				monthPart.value = dtf.digits(twoDigit(int(t.Month())))
			}
			if dayPart != nil && day == "numeric" && monthPart != nil {
				dayPart.value = dtf.digits(twoDigit(t.Day()))
			}
		}
		for _, p := range order {
			if p == nil {
				continue
			}
			if len(out) > 0 {
				out = append(out, lit(sep))
			}
			out = append(out, *p)
		}
	default:
		for _, p := range []*intlPart{dayPart, yearPart} {
			if p != nil {
				if len(out) > 0 {
					out = append(out, lit(" "))
				}
				out = append(out, *p)
			}
		}
	}
	if eraPart != nil {
		if len(out) > 0 {
			out = append(out, lit(" "))
		}
		out = append(out, *eraPart)
	}
	if weekdayPart != nil {
		if len(out) == 0 {
			out = []intlPart{*weekdayPart}
		} else if dayPart != nil && monthPart == nil && yearPart == nil {
			out = append(out, lit(" "), *weekdayPart)
		} else {
			sep := ", "
			if !usStyle && textual {
				sep = " "
			}
			out = append([]intlPart{*weekdayPart, lit(sep)}, out...)
		}
	}
	return out
}

func (dtf *intlDateTimeFormat) timeParts(t time.Time, hour, minute, second string, fsd int, dayPeriod, tzName string) []intlPart {
	lit := func(s string) intlPart { return intlPart{"literal", s} }
	var out []intlPart
	hc := dtf.hourCycle
	if hc == "" {
		hc = "h12"
		if !intlH12Regions[dtf.region] {
			hc = "h23"
		}
	}
	h := t.Hour()
	if hour != "" {
		display := h
		switch hc {
		case "h12":
			display = h % 12
			if display == 0 {
				display = 12
			}
		case "h11":
			display = h % 12
		case "h24":
			if display == 0 {
				display = 24
			}
		}
		s := strconv.Itoa(display)
		if hour == "2-digit" || hc == "h23" || hc == "h24" {
			s = twoDigit(display)
		}
		out = append(out, intlPart{"hour", dtf.digits(s)})
	}
	if minute != "" {
		s := strconv.Itoa(t.Minute())
		if hour != "" || second != "" || minute == "2-digit" {
			s = twoDigit(t.Minute())
		}
		if len(out) > 0 {
			out = append(out, lit(":"))
		}
		out = append(out, intlPart{"minute", dtf.digits(s)})
	}
	if second != "" || fsd > 0 {
		if second != "" {
			s := strconv.Itoa(t.Second())
			if minute != "" || second == "2-digit" {
				s = twoDigit(t.Second())
			}
			if len(out) > 0 {
				out = append(out, lit(":"))
			}
			out = append(out, intlPart{"second", dtf.digits(s)})
		}
		if fsd > 0 {
			ms := fmt.Sprintf("%03d", t.Nanosecond()/1e6)[:fsd]
			if len(out) > 0 {
				out = append(out, lit("."))
			}
			out = append(out, intlPart{"fractionalSecond", dtf.digits(ms)})
		}
	}
	twelve := hc == "h12" || hc == "h11"
	switch {
	case dayPeriod != "" && (hour == "" || twelve):
		if len(out) > 0 {
			out = append(out, lit(" "))
		}
		out = append(out, intlPart{"dayPeriod", intlFlexibleDayPeriod(t, dayPeriod)})
	case hour != "" && twelve:
		period := "AM"
		if h >= 12 {
			period = "PM"
		}
		out = append(out, lit(intlNNBSP), intlPart{"dayPeriod", period})
	}
	if tzName != "" {
		if len(out) > 0 {
			out = append(out, lit(" "))
		}
		out = append(out, intlPart{"timeZoneName", dtf.zoneName(t, tzName)})
	}
	return out
}

// intlFlexibleDayPeriod is CLDR English "flexible" day periods.
func intlFlexibleDayPeriod(t time.Time, width string) string {
	h, m := t.Hour(), t.Minute()
	if h == 12 && m == 0 && t.Second() == 0 {
		if width == "narrow" {
			return "n"
		}
		return "noon"
	}
	switch {
	case h >= 6 && h < 12:
		return "in the morning"
	case h >= 12 && h < 18:
		return "in the afternoon"
	case h >= 18 && h < 21:
		return "in the evening"
	}
	return "at night"
}

var intlUSZoneAbbrevs = map[string][2]string{
	"EST": {"Eastern Standard Time", "ET"}, "EDT": {"Eastern Daylight Time", "ET"},
	"CST": {"Central Standard Time", "CT"}, "CDT": {"Central Daylight Time", "CT"},
	"MST": {"Mountain Standard Time", "MT"}, "MDT": {"Mountain Daylight Time", "MT"},
	"PST": {"Pacific Standard Time", "PT"}, "PDT": {"Pacific Daylight Time", "PT"},
	"AKST": {"Alaska Standard Time", "AKT"}, "AKDT": {"Alaska Daylight Time", "AKT"},
	"HST": {"Hawaii-Aleutian Standard Time", "HST"}, "HDT": {"Hawaii-Aleutian Daylight Time", "HST"},
}

var intlZoneLongNames = map[string]string{
	"GMT": "Greenwich Mean Time", "BST": "British Summer Time", "IST": "India Standard Time",
	"CET": "Central European Standard Time", "CEST": "Central European Summer Time",
	"EET": "Eastern European Standard Time", "EEST": "Eastern European Summer Time",
	"WET": "Western European Standard Time", "WEST": "Western European Summer Time",
	"JST": "Japan Standard Time", "KST": "Korean Standard Time",
	"AEST": "Australian Eastern Standard Time", "AEDT": "Australian Eastern Daylight Time",
	"NZST": "New Zealand Standard Time", "NZDT": "New Zealand Daylight Time",
}

func (dtf *intlDateTimeFormat) zoneName(t time.Time, style string) string {
	abbrev, offset := t.Zone()
	gmt := func(long bool) string {
		if offset == 0 {
			return "GMT"
		}
		sign := "+"
		o := offset
		if o < 0 {
			sign, o = "-", -o
		}
		h, m := o/3600, (o%3600)/60
		if long {
			return fmt.Sprintf("GMT%s%02d:%02d", sign, h, m)
		}
		if m != 0 {
			return fmt.Sprintf("GMT%s%d:%02d", sign, h, m)
		}
		return fmt.Sprintf("GMT%s%d", sign, h)
	}
	if intlIsUTCAlias(dtf.timeZone) {
		switch style {
		case "long":
			return "Coordinated Universal Time"
		case "short":
			return "UTC"
		}
		return "GMT"
	}
	us, isUS := intlUSZoneAbbrevs[abbrev]
	switch style {
	case "short":
		if isUS || (abbrev == "GMT" && offset == 0) {
			return abbrev
		}
		return gmt(false)
	case "long":
		if isUS {
			return us[0]
		}
		if name, ok := intlZoneLongNames[abbrev]; ok && !strings.HasPrefix(dtf.timeZone, "+") && !strings.HasPrefix(dtf.timeZone, "-") {
			return name
		}
		return gmt(true)
	case "shortOffset":
		return gmt(false)
	case "longOffset":
		return gmt(true)
	case "shortGeneric":
		if isUS {
			return us[1]
		}
		return gmt(false)
	case "longGeneric":
		if isUS {
			return strings.Replace(strings.Replace(us[0], " Standard", "", 1), " Daylight", "", 1)
		}
		return gmt(true)
	}
	return abbrev
}

// rangeParts is FormatDateTimeRange(ToParts): one rendering when both ends
// look the same, else "start – end" with thin spaces.
func (dtf *intlDateTimeFormat) rangeParts(x, y float64) ([]intlPart, []string) {
	xp, yp := dtf.partsFor(x), dtf.partsFor(y)
	if intlJoinParts(xp) == intlJoinParts(yp) {
		src := make([]string, len(xp))
		for i := range src {
			src[i] = "shared"
		}
		return xp, src
	}
	if parts, src, ok := dtf.collapsedTextualRange(x, y); ok {
		return parts, src
	}
	var parts []intlPart
	var src []string
	for _, p := range xp {
		parts, src = append(parts, p), append(src, "startRange")
	}
	parts, src = append(parts, intlPart{"literal", intlThinSpace + "–" + intlThinSpace}), append(src, "shared")
	for _, p := range yp {
		parts, src = append(parts, p), append(src, "endRange")
	}
	return parts, src
}

// collapsedTextualRange writes a US-style "Mon D, Y" range with the shared
// year (and month) once, as ICU's interval formats do: "Jan 3 – 5, 2019",
// "Jan 3 – Mar 4, 2019".
func (dtf *intlDateTimeFormat) collapsedTextualRange(x, y float64) ([]intlPart, []string, bool) {
	textual := dtf.month == "long" || dtf.month == "short" || dtf.month == "narrow"
	if dtf.dateStyle == "medium" || dtf.dateStyle == "long" {
		textual = true
	}
	if !textual || !intlUSStyleRegions[dtf.region] || dtf.timeStyle != "" || dtf.weekday != "" || dtf.era != "" ||
		dtf.hour != "" || dtf.minute != "" || dtf.second != "" || dtf.fsd != 0 || dtf.dayPeriod != "" || dtf.timeZoneName != "" ||
		dtf.dateStyle == "full" || dtf.dateStyle == "short" {
		return nil, nil, false
	}
	monthStyle, hasDay, hasYear := dtf.month, dtf.day != "", dtf.year != ""
	if dtf.dateStyle != "" {
		monthStyle, hasDay, hasYear = map[string]string{"medium": "short", "long": "long"}[dtf.dateStyle], true, true
	}
	if !hasDay || !hasYear {
		return nil, nil, false
	}
	a, b := time.UnixMilli(int64(x)).In(dtf.loc), time.UnixMilli(int64(y)).In(dtf.loc)
	if a.Year() != b.Year() || a.Year() <= 0 {
		return nil, nil, false
	}
	dayFmt := dtf.day
	if dayFmt == "" {
		dayFmt = "numeric"
	}
	start := dtf.dateParts(a, "", "", "", monthStyle, dayFmt)
	var end []intlPart
	if a.Month() == b.Month() {
		end = dtf.dateParts(b, "", "", "", "", dayFmt)
	} else {
		end = dtf.dateParts(b, "", "", "", monthStyle, dayFmt)
	}
	yearStyle := dtf.year
	if yearStyle == "" {
		yearStyle = "numeric"
	}
	yearParts := dtf.dateParts(b, "", "", yearStyle, "", "")
	var parts []intlPart
	var src []string
	for _, p := range start {
		parts, src = append(parts, p), append(src, "startRange")
	}
	parts, src = append(parts, intlPart{"literal", intlThinSpace + "–" + intlThinSpace}), append(src, "shared")
	for _, p := range end {
		parts, src = append(parts, p), append(src, "endRange")
	}
	parts, src = append(parts, intlPart{"literal", ", "}), append(src, "shared")
	for _, p := range yearParts {
		parts, src = append(parts, p), append(src, "shared")
	}
	return parts, src, true
}

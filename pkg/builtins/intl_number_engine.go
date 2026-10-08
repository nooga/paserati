package builtins

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

// ---- Exact decimal values ----

// intlDecimal is an exact Intl mathematical value: coef × 10^-scale, plus
// the non-finite and negative-zero cases ToIntlMathematicalValue keeps.
type intlDecimal struct {
	neg   bool
	coef  *big.Int // non-negative
	scale int      // value = coef / 10^scale
	nan   bool
	inf   bool
}

var bigTen = big.NewInt(10)

func intlPow10(n int) *big.Int {
	return new(big.Int).Exp(bigTen, big.NewInt(int64(n)), nil)
}

// intlDecimalFromFloat takes a Number's shortest round-trip decimal form,
// which is what ICU (and so every browser) formats: 1.005 is "1.005", not
// the binary value just below it.
func intlDecimalFromFloat(f float64) intlDecimal {
	switch {
	case math.IsNaN(f):
		return intlDecimal{nan: true}
	case math.IsInf(f, 0):
		return intlDecimal{inf: true, neg: f < 0}
	}
	d := intlDecimal{neg: math.Signbit(f), coef: new(big.Int)}
	if f == 0 {
		return d
	}
	s := strconv.FormatFloat(math.Abs(f), 'e', -1, 64) // d.ddddde±x
	mant, expStr, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(expStr)
	digits := strings.Replace(mant, ".", "", 1)
	d.coef.SetString(digits, 10)
	d.scale = len(digits) - 1 - exp
	return d.normalized()
}

func intlDecimalFromBigInt(b *big.Int) intlDecimal {
	return intlDecimal{neg: b.Sign() < 0, coef: new(big.Int).Abs(b)}
}

// intlDecimalFromString parses a StringNumericLiteral exactly (the String
// case of ToIntlMathematicalValue); ok is false for anything that isn't one,
// which the caller turns into NaN.
func intlDecimalFromString(s string) (intlDecimal, bool) {
	s = strings.TrimFunc(s, isJSWhitespaceOrLineTerminator)
	if s == "" {
		return intlDecimal{coef: new(big.Int)}, true
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			v, ok := new(big.Int).SetString(s[2:], base)
			if !ok || strings.ContainsAny(s[2:], "+-_") {
				return intlDecimal{}, false
			}
			return intlDecimal{coef: v}, true
		}
	}
	neg := false
	body := s
	if body[0] == '+' || body[0] == '-' {
		neg = body[0] == '-'
		body = body[1:]
	}
	if body == "Infinity" {
		return intlDecimal{inf: true, neg: neg}, true
	}
	mant, expPart := body, ""
	if i := strings.IndexAny(body, "eE"); i >= 0 {
		mant, expPart = body[:i], body[i+1:]
		if expPart == "" {
			return intlDecimal{}, false
		}
	}
	intPart, fracPart, hasDot := strings.Cut(mant, ".")
	if intPart == "" && fracPart == "" {
		return intlDecimal{}, false
	}
	if !hasDot {
		fracPart = ""
	}
	for _, part := range []string{intPart, fracPart} {
		for i := 0; i < len(part); i++ {
			if part[i] < '0' || part[i] > '9' {
				return intlDecimal{}, false
			}
		}
	}
	exp := 0
	if expPart != "" {
		e, err := strconv.Atoi(expPart)
		if err != nil {
			if len(expPart) > 1 && (expPart[0] == '+' || expPart[0] == '-') {
				// Huge exponents: saturate.
				allDigits := true
				for i := 1; i < len(expPart); i++ {
					allDigits = allDigits && expPart[i] >= '0' && expPart[i] <= '9'
				}
				if !allDigits {
					return intlDecimal{}, false
				}
				if expPart[0] == '-' {
					return intlDecimal{neg: neg, coef: new(big.Int)}, true
				}
				return intlDecimal{inf: true, neg: neg}, true
			}
			return intlDecimal{}, false
		}
		exp = e
	}
	digits := strings.TrimLeft(intPart+fracPart, "0")
	d := intlDecimal{neg: neg, coef: new(big.Int)}
	if digits == "" {
		return d, true
	}
	d.coef.SetString(digits, 10)
	d.scale = len(fracPart) - exp
	if d.scale < -100000 {
		return intlDecimal{inf: true, neg: neg}, true
	}
	return d.normalized(), true
}

func isJSWhitespaceOrLineTerminator(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', '\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000', 0xFEFF:
		return true
	}
	return r >= '\u2000' && r <= '\u200a'
}

// normalized drops trailing zeros from the coefficient.
func (d intlDecimal) normalized() intlDecimal {
	if d.coef == nil || d.coef.Sign() == 0 {
		d.scale = 0
		return d
	}
	q, r := new(big.Int), new(big.Int)
	for {
		q.QuoRem(d.coef, bigTen, r)
		if r.Sign() != 0 {
			return d
		}
		d.coef = new(big.Int).Set(q)
		d.scale--
	}
}

func (d intlDecimal) isZero() bool { return !d.nan && !d.inf && (d.coef == nil || d.coef.Sign() == 0) }

// magnitude is floor(log10(|d|)) for a non-zero finite value.
func (d intlDecimal) magnitude() int {
	return len(d.coef.String()) - 1 - d.scale
}

// shift multiplies by 10^n (percent style, compact/scientific scaling).
func (d intlDecimal) shift(n int) intlDecimal {
	if d.nan || d.inf || d.isZero() {
		return d
	}
	d.scale -= n
	return d
}

// roundToIncrement rounds |d| to a multiple of inc × 10^-fd with the given
// rounding mode (which looks at the sign for ceil/floor).
func (d intlDecimal) roundToIncrement(fd int, inc int64, mode string) intlDecimal {
	if d.nan || d.inf || d.isZero() {
		return d
	}
	// units = coef × 10^(fd - scale) as an exact fraction num/den.
	num := new(big.Int).Set(d.coef)
	den := big.NewInt(1)
	if k := fd - d.scale; k >= 0 {
		num.Mul(num, intlPow10(k))
	} else {
		den = intlPow10(-k)
	}
	den.Mul(den, big.NewInt(inc))
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	if r.Sign() != 0 && intlRoundUp(q, r, den, d.neg, mode) {
		q.Add(q, big.NewInt(1))
	}
	q.Mul(q, big.NewInt(inc))
	return intlDecimal{neg: d.neg, coef: q, scale: fd}.normalized()
}

// intlRoundUp decides, for |x| = q + r/den with 0 < r < den, whether the
// magnitude rounds away from zero.
func intlRoundUp(q, r, den *big.Int, neg bool, mode string) bool {
	twice := new(big.Int).Lsh(r, 1)
	half := twice.Cmp(den) // <0 below half, 0 exactly half, >0 above
	switch mode {
	case "ceil":
		return !neg
	case "floor":
		return neg
	case "expand":
		return true
	case "trunc":
		return false
	}
	if half != 0 {
		return half > 0
	}
	switch mode {
	case "halfCeil":
		return !neg
	case "halfFloor":
		return neg
	case "halfTrunc":
		return false
	case "halfEven":
		return q.Bit(0) == 1
	}
	return true // halfExpand
}

// ---- Raw number formatting (ECMA-402 FormatNumericToString) ----

type intlRawNumber struct {
	intDigits  string // no grouping, at least one digit
	fracDigits string
	magnitude  int // rounding magnitude: the position of the last digit kept
}

// toRawFixed implements ToRawFixed.
func (d intlDecimal) toRawFixed(minFD, maxFD int, inc int64, mode string) (intlRawNumber, intlDecimal) {
	r := d.roundToIncrement(maxFD, inc, mode)
	return r.digitsWithFraction(minFD, maxFD), r
}

// toRawPrecision implements ToRawPrecision.
func (d intlDecimal) toRawPrecision(minSD, maxSD int, mode string) (intlRawNumber, intlDecimal) {
	if d.isZero() {
		frac := strings.Repeat("0", minSD-1)
		return intlRawNumber{intDigits: "0", fracDigits: frac, magnitude: -(maxSD - 1)}, d
	}
	e := d.magnitude()
	fd := maxSD - 1 - e
	r := d.roundToIncrement(fd, 1, mode)
	if !r.isZero() && r.magnitude() > e { // 9.99 -> 10.0: one more integer digit
		e = r.magnitude()
		fd = maxSD - 1 - e
		r = d.roundToIncrement(fd, 1, mode)
	}
	minFrac := minSD - 1 - e
	raw := r.digitsWithFraction(minFrac, fd)
	raw.magnitude = e - maxSD + 1
	return raw, r
}

// digitsWithFraction renders |d| with between minFD and maxFD fraction
// digits (trailing zeros beyond minFD stripped). maxFD may be negative
// (rounding to tens etc.); the value is already rounded.
func (d intlDecimal) digitsWithFraction(minFD, maxFD int) intlRawNumber {
	if minFD < 0 {
		minFD = 0
	}
	coef := "0"
	if d.coef != nil && d.coef.Sign() != 0 {
		coef = d.coef.String()
	}
	scale := d.scale
	var intPart, frac string
	if scale <= 0 {
		intPart = coef + strings.Repeat("0", -scale)
	} else if scale >= len(coef) {
		intPart = "0"
		frac = strings.Repeat("0", scale-len(coef)) + coef
	} else {
		intPart = coef[:len(coef)-scale]
		frac = coef[len(coef)-scale:]
	}
	frac = strings.TrimRight(frac, "0")
	if len(frac) < minFD {
		frac += strings.Repeat("0", minFD-len(frac))
	}
	return intlRawNumber{intDigits: intPart, fracDigits: frac, magnitude: -maxFD}
}

// ---- Locale number symbols ----

type intlNumberSymbols struct {
	decimal, group string
	minus, plus    string
	percentPrefix  string // e.g. "%" in tr
	percentSuffix  string // e.g. "%" in en, " %" in de
	primaryGroup   int
	secondaryGroup int
	minGrouping    int
	currencyBefore bool   // ¤ precedes the number
	currencySpace  string // between ¤ and the number ("" or NBSP)
}

var intlSymbolsCache sync.Map // language tag string -> *intlNumberSymbols

// intlCurrencySuffixLanguages put the currency after the number with a
// space (CLDR "#,##0.00 ¤"); intlCurrencySpacedPrefix put it before with
// one ("¤ #,##0.00"). Everything else is "¤#,##0.00".
var intlCurrencySuffixLanguages = map[string]bool{
	"de": true, "fr": true, "es": true, "it": true, "ru": true, "pl": true, "cs": true, "sk": true,
	"sv": true, "fi": true, "nb": true, "no": true, "nn": true, "da": true, "uk": true, "hu": true,
	"ro": true, "sl": true, "hr": true, "bs": true, "sr": true, "bg": true, "lt": true, "lv": true,
	"et": true, "el": true, "ca": true, "be": true, "kk": true, "az": true, "is": true, "pt-PT": true,
	"vi": true, "he": true, "ar": true, "fa": true, "hy": true, "ka": true, "mk": true, "sq": true,
}
var intlCurrencySpacedPrefix = map[string]bool{
	"nl": true, "pt": true, "de-CH": true, "de-AT": true, "de-LI": true, "it-CH": true, "fr-CH": true, "en-ZA": true,
	"ur": true, "ps": true, "id": false,
}

// intlNumberSymbolsFor reads a locale's CLDR number symbols out of
// x/text by formatting probes, and caches them.
func intlNumberSymbolsFor(locale string) *intlNumberSymbols {
	if v, ok := intlSymbolsCache.Load(locale); ok {
		return v.(*intlNumberSymbols)
	}
	tag, err := language.Parse(locale)
	if err != nil {
		tag = language.AmericanEnglish
	}
	p := message.NewPrinter(tag)
	sym := &intlNumberSymbols{decimal: ".", group: ",", minus: "-", plus: "+", percentSuffix: "%", primaryGroup: 3, secondaryGroup: 3, minGrouping: 1, currencyBefore: true}

	// "1,234,567.891" -> group ",", decimal "."; Indian "12,34,567.891".
	probe := p.Sprint(number.Decimal(1234567.891, number.MinFractionDigits(3)))
	if i := strings.Index(probe, "891"); i > 0 {
		head := probe[:i]
		if j := strings.LastIndex(head, "567"); j >= 0 {
			sym.decimal = head[j+3:]
			intPart := head[:j+3]
			groups := intlSplitNonDigits(intPart)
			if len(groups) > 1 {
				sym.group = intlFirstSeparator(intPart)
				sym.primaryGroup = len(groups[len(groups)-1])
				sym.secondaryGroup = len(groups[len(groups)-2])
				if len(groups) == 2 {
					sym.secondaryGroup = sym.primaryGroup
				}
			} else {
				sym.group = ""
			}
		}
	}
	if !strings.ContainsAny(p.Sprint(number.Decimal(1234)), sym.group) || sym.group == "" {
		sym.minGrouping = 2
	}
	if neg := p.Sprint(number.Decimal(-1)); strings.HasSuffix(neg, "1") {
		sym.minus = strings.TrimSuffix(neg, "1")
	}
	if pct := p.Sprint(number.Percent(0.25)); strings.Contains(pct, "25") {
		i := strings.Index(pct, "25")
		sym.percentPrefix, sym.percentSuffix = pct[:i], pct[i+2:]
	}

	base, _ := tag.Base()
	region, _ := tag.Region()
	langRegion := base.String() + "-" + region.String()
	switch {
	case intlCurrencySpacedPrefix[langRegion] || (intlCurrencySpacedPrefix[base.String()] && !intlCurrencySuffixLanguages[langRegion]):
		sym.currencyBefore, sym.currencySpace = true, "\u00a0"
	case intlCurrencySuffixLanguages[langRegion] || intlCurrencySuffixLanguages[base.String()]:
		sym.currencyBefore, sym.currencySpace = false, "\u00a0"
	}
	intlSymbolsCache.Store(locale, sym)
	return sym
}

func intlSplitNonDigits(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' })
}

func intlFirstSeparator(s string) string {
	start := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
	if start < 0 {
		return ""
	}
	end := start + strings.IndexFunc(s[start:], func(r rune) bool { return r >= '0' && r <= '9' })
	if end < start {
		end = len(s)
	}
	return s[start:end]
}

// intlCurrencySymbol is the locale's symbol (or narrow symbol) for an ISO
// 4217 code, falling back to the code itself.
func intlCurrencySymbol(locale, code string, narrow bool) string {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return code
	}
	tag, err := language.Parse(locale)
	if err != nil {
		tag = language.AmericanEnglish
	}
	p := message.NewPrinter(tag)
	if narrow {
		return p.Sprint(currency.NarrowSymbol(unit))
	}
	return p.Sprint(currency.Symbol(unit))
}

// intlCurrencyDigits is CurrencyDigits: the ISO 4217 minor unit, 2 when
// unknown.
func intlCurrencyDigits(code string) int {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 2
	}
	scale, _ := currency.Standard.Rounding(unit)
	return scale
}

// intlCurrencyNames are English display names (singular, plural) for
// currencyDisplay: "name"; other codes display as the code.
var intlCurrencyNames = map[string][2]string{
	"USD": {"US dollar", "US dollars"}, "EUR": {"euro", "euros"}, "GBP": {"British pound", "British pounds"},
	"JPY": {"Japanese yen", "Japanese yen"}, "CNY": {"Chinese yuan", "Chinese yuan"}, "CHF": {"Swiss franc", "Swiss francs"},
	"CAD": {"Canadian dollar", "Canadian dollars"}, "AUD": {"Australian dollar", "Australian dollars"},
	"INR": {"Indian rupee", "Indian rupees"}, "KRW": {"South Korean won", "South Korean won"},
	"MXN": {"Mexican peso", "Mexican pesos"}, "BRL": {"Brazilian real", "Brazilian reals"},
	"RUB": {"Russian ruble", "Russian rubles"}, "SEK": {"Swedish krona", "Swedish kronor"},
	"NOK": {"Norwegian krone", "Norwegian kroner"}, "DKK": {"Danish krone", "Danish kroner"},
	"PLN": {"Polish zloty", "Polish zlotys"}, "CZK": {"Czech koruna", "Czech korunas"},
	"HKD": {"Hong Kong dollar", "Hong Kong dollars"}, "SGD": {"Singapore dollar", "Singapore dollars"},
	"NZD": {"New Zealand dollar", "New Zealand dollars"}, "ZAR": {"South African rand", "South African rand"},
	"TRY": {"Turkish lira", "Turkish lira"}, "ILS": {"Israeli new shekel", "Israeli new shekels"},
	"BTC": {"bitcoin", "bitcoins"},
}

// ---- Grouping ----

// intlGroup inserts the locale's group separator into a plain integer
// digit string per the grouping strategy ("always", "auto", "min2", or ""
// for none).
func intlGroup(intDigits string, sym *intlNumberSymbols, useGrouping string) []string {
	if useGrouping == "" || sym.group == "" || len(intDigits) <= sym.primaryGroup {
		return []string{intDigits}
	}
	minGrouping := 1
	switch useGrouping {
	case "auto":
		minGrouping = sym.minGrouping
	case "min2":
		minGrouping = 2
	}
	if len(intDigits) < sym.primaryGroup+minGrouping {
		return []string{intDigits}
	}
	var groups []string
	end := len(intDigits)
	start := end - sym.primaryGroup
	groups = append(groups, intDigits[start:end])
	end = start
	for end > 0 {
		start = end - sym.secondaryGroup
		if start < 0 {
			start = 0
		}
		groups = append([]string{intDigits[start:end]}, groups...)
		end = start
	}
	return groups
}

// ---- Parts ----

type intlPart struct {
	typ, value string
}

func intlJoinParts(parts []intlPart) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.value)
	}
	return b.String()
}

// intlCharCount is the length in code points (for pattern decisions).
func intlCharCount(s string) int { return utf8.RuneCountInString(s) }

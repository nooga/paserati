// expect: 1,234.5|1.234.567,891|12,34,567.891|-$1,234.50|($5.00)|26%|1.2M|1.235E5|50 km/h|+3|12,345,678,901,234,567,890|1.01|3–5|1,234.5,2
// Intl.NumberFormat and toLocaleString (#624).
const nf = (l: string, o?: any) => new Intl.NumberFormat(l, o);
[
  (1234.5).toLocaleString("en-US"),
  nf("de-DE").format(1234567.891),
  nf("en-IN").format(1234567.891),
  nf("en", { style: "currency", currency: "USD" }).format(-1234.5),
  nf("en", { style: "currency", currency: "USD", currencySign: "accounting" }).format(-5),
  nf("en", { style: "percent" }).format(0.256),
  nf("en", { notation: "compact" }).format(1234567),
  nf("en", { notation: "scientific" }).format(123456),
  nf("en", { style: "unit", unit: "kilometer-per-hour" }).format(50),
  nf("en", { signDisplay: "always" }).format(3),
  (12345678901234567890n).toLocaleString("en"),
  nf("en", { maximumFractionDigits: 2 }).format(1.005),
  nf("en").formatRange(3, 5),
  [1234.5, 2].toLocaleString("en"),
].join("|");

// expect: 1/2/2024, 8:04:05 PM|Tuesday, January 2, 2024 at 8:04:05 PM UTC|02/01/2024|20:04|Jan 3 – 5, 2019|en-US|1/2/2024
// Intl.DateTimeFormat and Date toLocale* (#624). U+202F before AM/PM and
// U+2009 around the range dash are normalized to spaces for readability.
const d = new Date(Date.UTC(2024, 0, 2, 20, 4, 5));
const utc = { timeZone: "UTC" };
const norm = (s: string) => s.replace(/[  ]/g, " ");
[
  d.toLocaleString("en-US", utc),
  new Intl.DateTimeFormat("en-US", { ...utc, dateStyle: "full", timeStyle: "long" }).format(d),
  new Intl.DateTimeFormat("en-GB", utc).format(d),
  d.toLocaleTimeString("en-US", { ...utc, hour: "numeric", minute: "2-digit", hour12: false }),
  new Intl.DateTimeFormat("en-US", { ...utc, month: "short", day: "numeric", year: "numeric" })
    .formatRange(Date.UTC(2019, 0, 3), Date.UTC(2019, 0, 5)),
  new Intl.DateTimeFormat("de-DE").resolvedOptions().locale,
  d.toLocaleDateString("en-US", utc),
].map(norm).join("|");

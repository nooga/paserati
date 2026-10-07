// expect: 1546300800000 1546300800000 true true 1546300800500 8640000000000000 NaN NaN NaN
// #609: Date.parse round-trips toUTCString/toISOString and validates ISO strings
const d = new Date(Date.UTC(2019, 0, 15, 13, 45, 30, 123));
[
  Date.parse("Tue, 01 Jan 2019 00:00:00 GMT"),
  Date.parse("01 Jan 2019 00:00:00 GMT"),
  Date.parse(d.toUTCString()) === d.getTime() - 123,
  Date.parse(d.toISOString()) === d.getTime(),
  Date.parse("2019-01-01T00:00:00.5Z"),
  Date.parse("+275760-09-13T00:00:00Z"),
  Date.parse("+275760-09-13T00:00:00.001Z"),
  Date.parse("2019-13-01"),
  Date.parse("-000000-01-01T00:00:00Z"),
].join(" ");

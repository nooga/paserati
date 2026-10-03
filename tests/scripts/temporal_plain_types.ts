// Temporal plain types: construction, arithmetic with month-end clamping,
// differences, rounding and formatting, all with type checking on.
// expect: 2024-02-29|P1Y2M4D|2024-03-10T12:30:00|12:35:00|03-10|2024-03
const jan31 = Temporal.PlainDate.from("2024-01-31");
const clamped = jan31.add({ months: 1 }).toString();
const diff = Temporal.PlainDate.from("2020-01-01").until("2021-03-05", { largestUnit: "years" }).toString();
const dt = new Temporal.PlainDateTime(2024, 3, 10, 12, 30).toString();
const t = Temporal.PlainTime.from("12:34:56").round({ smallestUnit: "minute", roundingMode: "ceil" }).toString();
const md = Temporal.PlainMonthDay.from({ month: 3, day: 10 }).toString();
const ym = Temporal.PlainYearMonth.from("2024-03").toString();
[clamped, diff, dt, t, md, ym].join("|");

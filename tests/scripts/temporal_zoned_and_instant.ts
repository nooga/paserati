// ZonedDateTime across a DST change, Instant conversion and Duration rounding.
// expect: 2024-03-11T12:00:00-04:00[America/New_York]|2024-03-10T09:00:00+09:00[Asia/Tokyo]|P5DT10H20M|23|469200
const ny = Temporal.ZonedDateTime.from("2024-03-10T00:00:00[America/New_York]");
const next = ny.add({ days: 1, hours: 12 }).toString();
const tokyo = Temporal.Instant.from("2024-03-10T00:00:00Z").toZonedDateTimeISO("Asia/Tokyo").toString();
const rounded = Temporal.Duration.from({ hours: 130, minutes: 20 }).round({ largestUnit: "day" }).toString();
const hoursInDay = ny.hoursInDay;
const seconds = Temporal.Duration.from({ hours: 130, minutes: 20 }).total({ unit: "second" });
[next, tokyo, rounded, hoursInDay, seconds].join("|");

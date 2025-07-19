// Test new Date methods
// expect: All date tests passed

// Test UTC getters
let d = new Date(Date.UTC(2024, 11, 25, 15, 30, 45, 123));
let utcTests = 
    d.getUTCFullYear() === 2024 &&
    d.getUTCMonth() === 11 &&
    d.getUTCDate() === 25 &&
    d.getUTCDay() === 3 &&
    d.getUTCHours() === 15 &&
    d.getUTCMinutes() === 30 &&
    d.getUTCSeconds() === 45 &&
    d.getUTCMilliseconds() === 123;

// Test setTime
let d2 = new Date();
d2.setTime(1000000000000);
let setTimeTest = d2.toISOString() === "2001-09-09T01:46:40.000Z";

// Test locale methods existence
let localeTests = 
    typeof d.toLocaleString() === "string" &&
    typeof d.toLocaleDateString() === "string" &&
    typeof d.toLocaleTimeString() === "string";

// Test toJSON
let d3 = new Date(Date.UTC(2024, 0, 1, 12, 0, 0));
let jsonTest = d3.toJSON() === "2024-01-01T12:00:00.000Z";

// Test invalid date toJSON
let invalidDate = new Date("invalid");
let invalidJsonTest = invalidDate.toJSON() === null;

// Test multi-param setters
let d4 = new Date(2020, 0, 1);
d4.setHours(10, 20, 30, 400);
let multiParamTest = 
    d4.getHours() === 10 &&
    d4.getMinutes() === 20 &&
    d4.getSeconds() === 30 &&
    d4.getMilliseconds() === 400;

// Test UTC setters
let d5 = new Date(Date.UTC(2020, 0, 1));
d5.setUTCFullYear(2024, 11, 31);
d5.setUTCHours(23, 59, 59, 999);
let utcSetterTest = d5.toISOString() === "2024-12-31T23:59:59.999Z";

// Test timezone offset
let timezoneTest = typeof d.getTimezoneOffset() === "number";

if (utcTests && setTimeTest && localeTests && jsonTest && invalidJsonTest && 
    multiParamTest && utcSetterTest && timezoneTest) {
    console.log("All date tests passed");
} else {
    console.log("Some date tests failed");
}
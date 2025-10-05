// Test mixing top-level await with regular code
// expect: success

let state = "init";

const delay = Promise.resolve("async");
state = "before";
const asyncValue = await delay;
state = "after";

const result = (state === "after" && asyncValue === "async") ? "success" : "fail";
result;

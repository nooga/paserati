// skip-typecheck
// expect: true
// paserati#440 follow-up: assignment to any non-writable global (not just
// `undefined`) was incorrectly performing the write in sloppy mode - only
// strict mode's TypeError path checked writability; the sloppy path fell
// through to the store unconditionally. Real Node: `NaN = 5; console.log(NaN)`
// still prints NaN (assignment is a silent no-op, per Set/PutValue on a
// non-writable data property with a false Throw flag).
NaN = 5;
Number.isNaN(NaN);

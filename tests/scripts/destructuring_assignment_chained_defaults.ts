// expect: false-true
// Test chained assignment in destructuring defaults
let flag1 = false, flag2 = false;
let _: any;
let vals = [14];
[ _ = flag1 = true, _ = flag2 = true ] = vals;
flag1 + "-" + flag2;

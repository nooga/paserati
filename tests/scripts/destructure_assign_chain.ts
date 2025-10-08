// expect: [1, 2, 3]
// Test that destructuring assignment returns the RHS value
var x, y, z;
var vals = [1, 2, 3];
var result = [x, y, z] = vals;
result;

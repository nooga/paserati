// expect: a:1,b:2,c:3
// no-typecheck
// Test that dynamic property access with variable keys works correctly in loops
// This tests the inline cache property name validation fix
var obj = { a: 1, b: 2, c: 3 };
var keys = ["a", "b", "c"];
var result = [];
for (var i = 0; i < keys.length; i++) {
    result.push(keys[i] + ":" + obj[keys[i]]);
}
result.join(",");

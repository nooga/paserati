// Test Function.apply with spread in array
function test() {
  console.log("args:", arguments.length);
  console.log("arg0:", arguments[0]);
  console.log("arg1:", arguments[1]);
  console.log("arg2:", arguments[2]);
}
test.apply(null, [1, 2, 3, ...[]]);
// expect: args: 3

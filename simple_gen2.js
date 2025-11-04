var x = 0;
function foo() {
  console.log("foo called");
  return 42;
}
var obj = {
  *method() {
    console.log("line 1");
    var result = foo();
    console.log("line 2, result =", result);
    x = 1;
    console.log("line 3");
  }
};
obj.method().next();
console.log("x =", x);

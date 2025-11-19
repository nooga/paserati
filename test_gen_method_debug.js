var obj = {
  *gen([x]) {
    console.log("Inside generator, x =", x);
    yield 1;
  }
};

var iter = {};
iter[Symbol.iterator] = function() {
  console.log("Symbol.iterator called");
  throw new Error("Iterator failed");
};

console.log("Before call");
try {
  var result = obj.gen(iter);
  console.log("Generator created, result =", result);
  console.log("About to call next()");
  result.next();
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
console.log("After try-catch");

// Test generator as object method with destructuring exception
var obj = {
  *gen([x]) {
    console.log("Inside generator, x =", x);
  }
};

var iter = {};
iter[Symbol.iterator] = function() {
  throw new Error("Iterator failed");
};

console.log("Before call");
try {
  obj.gen(iter);
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}
console.log("After try-catch");

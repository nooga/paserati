// Test if try-catch can catch exceptions during generator creation

function *gen([x]) {
  // Parameter destructuring will fail if argument doesn't have Symbol.iterator
  yield x;
}

try {
  console.log("About to create generator with bad argument");
  const g = gen(123); // 123 is not iterable
  console.log("FAIL - should have thrown");
} catch (e) {
  console.log("SUCCESS - caught:", e.message);
}

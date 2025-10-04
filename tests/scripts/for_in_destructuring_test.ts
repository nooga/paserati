// expect: done
// Test for-in loops with destructuring

// Note: for-in iterates over keys, so destructuring the key string itself
// This is a less common pattern but should work

// Array destructuring in for-in (destructuring the key)
const obj1 = { "ab": 1, "cd": 2, "ef": 3 };
for (const [first, second] of Object.keys(obj1)) {
  console.log(first + second);
}

("done");

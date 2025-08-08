var threw = false;

try {
  assert(false);
} catch(err) {
  threw = true;
  console.log("Caught error:", err.name, err.message);
  console.log("Constructor check:", err.constructor === Test262Error);
}

console.log("Threw:", threw);
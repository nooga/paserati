// expect: 15
// Regression guard for the tdz_*_forward_ref.ts fixes: normal (non-TDZ-
// violating) let/const usage inside try/catch/finally bodies - including a
// finally block, and a closure capturing a try-scoped binding - must keep
// working exactly as before.
function outer(): number {
  let closed: () => number = () => 0;
  try {
    let a = 10;
    closed = () => a;
  } finally {
    // finally runs regardless
  }
  return closed();
}
let result = outer();
try {
  let b = 5;
  result += b;
} catch (e) {
  result = -1;
}
result;

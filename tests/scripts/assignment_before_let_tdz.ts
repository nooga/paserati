// expect: rhs,ReferenceError,ReferenceError,ReferenceError 3
// skip-typecheck
// Assigning a let binding before its declaration runs throws ReferenceError:
// after the RHS for `=`, before it for compound operators. Matches Node.
function f() {
  var log = [];
  try { y = (log.push("rhs"), 5); } catch (e) { log.push(e.name); }
  try { y += (log.push("rhs2"), 1); } catch (e) { log.push(e.name); }
  try { y ||= 1; } catch (e) { log.push(e.name); }
  let y = 1; y = 2; y += 1;
  return log.join() + " " + y;
}
f();

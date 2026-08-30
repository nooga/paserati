// expect: caught
// A TypeError thrown for "not a constructor" during `new` must be catchable
// by a surrounding try/catch, not abort the whole script (issue #65).
let r = "not caught";
try {
  new Math();
} catch (e) {
  r = "caught";
}
r;

// expect: caught
// An array element turned into an accessor via Object.defineProperty, whose
// getter throws, must have its exception catchable by a surrounding
// try/catch (issue #65 followup: stale frame.ip made unwindException search
// for a handler at the wrong PC and miss a real one).
const arr = [1, 2, 3];
Object.defineProperty(arr, "0", {
  get() {
    throw new TypeError("nope");
  },
});
let r = "not caught";
try {
  arr[0];
} catch (e) {
  r = "caught";
}
r;

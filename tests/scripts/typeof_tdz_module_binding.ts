// expect: Cannot access 'later' before initialization|number
// typeof on a let/const binding evaluates the reference first, so inside the
// temporal dead zone it throws like any other read, and after initialization
// it reports the real type. Both used to come back "undefined" for a module's
// own top-level binding referenced from a hoisted function (#192), because
// the lookup used the bare name while the binding lives under the module key.
function probe(): string {
  return typeof later;
}
let inTdz: string;
try {
  inTdz = probe();
} catch (e) {
  inTdz = (e as Error).message;
}
let later = 1;
`${inTdz}|${probe()}`;

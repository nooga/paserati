// expect: 5
// A const made by Symbol() or Symbol.for() has a unique symbol type, a unit
// type: comparing with it narrows like comparing with a literal.
function memo<T>(make: () => T) {
  const unset = Symbol.for("memo/unset");
  let value: T | typeof unset = unset;
  return (): T => {
    if (value !== unset) {
      return value;
    }
    value = make();
    return value;
  };
}
const get = memo(() => 5);
get();

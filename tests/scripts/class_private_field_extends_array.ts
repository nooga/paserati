// no-typecheck
// Regression test for #478: a class with a private instance field that
// extends a built-in with real internal state (Array, Map, Set) must be
// constructible, unlike a PlainObject-based class, private fields for these
// live in a side table (see pkg/vm/properties_table.go) since their runtime
// representation isn't a PlainObject.
//
// The type checker doesn't yet model `class extends Array/Map/Set` at all
// (a separate, pre-existing gap unrelated to private fields - the same
// TS2339 "Property does not exist" shows up even with a plain public field),
// so this uses `// no-typecheck` like the runtime bug itself was found via
// (real Node packages compiled with type checking off).
// expect: done

class MyArray extends Array {
  #tag = "array-tag";
  getTag() { return this.#tag; }
}
const a = new MyArray(1, 2, 3);
if (a.getTag() !== "array-tag") throw new Error("array private field wrong");
if (a.length !== 3) throw new Error("array length wrong: " + a.length);

class MyMap extends Map {
  #tag = "map-tag";
  getTag() { return this.#tag; }
}
const m = new MyMap([["a", 1]]);
if (m.getTag() !== "map-tag") throw new Error("map private field wrong");
if (m.get("a") !== 1) throw new Error("map value wrong");

class MySet extends Set {
  #tag = "set-tag";
  getTag() { return this.#tag; }
}
const s = new MySet([1, 2, 3]);
if (s.getTag() !== "set-tag") throw new Error("set private field wrong");
if (s.size !== 3) throw new Error("set size wrong: " + s.size);

"done";

// expect: true
// Same bug family as paserati#176/#178 and array_sparse_index_read.ts, one
// level up: arrayLikeGet (pkg/builtins/array_generic.go) - the shared
// (value, exists, error) accessor every Array.prototype generic method
// (forEach/map/filter/some/every/at/indexOf/lastIndexOf/slice/...) uses to
// read element i - checked an own accessor first, then arr.HasIndex(i)
// (bounds-checks the dense .elements slice), and on a miss fell straight
// through to arrayIndexGetFromProto, which *only* walks the prototype
// chain. It never consulted the array's own .properties map, where
// ArrayDefineOwnProperty tracks a data property defined at an index past
// maxDenseArrayDefineIndex (2^24) - too large to grow .elements into (see
// array_props.go) - instead of materializing it there. That index is a
// perfectly genuine own property (arr[idx] and hasOwnProperty both see it
// fine - see array_sparse_index_read.ts), but every generic method backed
// by arrayLikeGet silently treated it as a hole.
//
// forEach/map/filter/some/every are all O(length), and the bug is only
// reachable past maxDenseArrayDefineIndex, so this necessarily costs a few
// seconds of wall time no matter how it's written - SPARSE_IDX is kept at
// exactly that bound (rather than some rounder, larger number), and the
// array is built once and reused (read-only) across those five checks, to
// keep it to one pass each rather than five separate arrays. at/indexOf/
// lastIndexOf/slice are included too since, called the way they are below,
// they exercise the same fixed code path in O(1) - cheap insurance for
// broader call-site coverage.
const checks: boolean[] = [];

const SPARSE_IDX = 16777217; // maxDenseArrayDefineIndex (16777216) + 1

const a: any[] = [1, 2, 3];
Object.defineProperty(a, String(SPARSE_IDX), {
  value: "sparse-val",
  writable: true,
  enumerable: true,
  configurable: true,
});

// Sanity: direct indexing and hasOwnProperty already worked (paserati#176).
checks.push(a[SPARSE_IDX] === "sparse-val");
checks.push((a as any).hasOwnProperty(String(SPARSE_IDX)));

// forEach must visit the sparse index with its real value, and must NOT
// visit a genuine hole below it (this fix must not turn every miss into a
// properties-map probe that invents a value that isn't there) - checked
// together in one pass since forEach is O(length) regardless.
//
// The callback is typed/returns `undefined` explicitly (rather than a
// plain arrow function) because Array.prototype.forEach's declared
// parameter type here is `(v, i?, arr?) => undefined`, and this checker
// does not currently accept a `void`-returning callback in its place -
// unlike real TypeScript, which allows that. Not this bug; a separate,
// narrower checker gap.
let found: any = "not-visited";
let visitedHole = false;
a.forEach(function (v: any, i?: number): undefined {
  if (i === SPARSE_IDX) found = v;
  if (i === 10) visitedHole = true;
  return undefined;
});
checks.push(found === "sparse-val");
checks.push(visitedHole === false);

// map must carry the value through at the same index.
const mapped = a.map(function (v) {
  return v;
});
checks.push(mapped[SPARSE_IDX] === "sparse-val");

// filter must see the element as present and matching.
const filtered = a.filter(function (v) {
  return v === "sparse-val";
});
checks.push(filtered.length === 1);
checks.push(filtered[0] === "sparse-val");

// some must find it.
checks.push(
  a.some(function (v) {
    return v === "sparse-val";
  }) === true
);

// every must see it (and so report false for a predicate it fails).
checks.push(
  a.every(function (v) {
    return v !== "sparse-val";
  }) === false
);

// at(-1): O(1) - resolves to exactly SPARSE_IDX since it's also the last
// index (length - 1).
checks.push(a.at(-1) === "sparse-val");

// lastIndexOf with no fromIndex starts scanning at length - 1, i.e.
// SPARSE_IDX itself: O(1), hits on the first step.
checks.push(a.lastIndexOf("sparse-val") === SPARSE_IDX);

// indexOf with fromIndex = SPARSE_IDX starts scanning there directly:
// O(1), hits on the first step.
checks.push(a.indexOf("sparse-val", SPARSE_IDX) === SPARSE_IDX);

// slice(SPARSE_IDX) copies exactly one element: O(1).
const sliced = a.slice(SPARSE_IDX);
checks.push(sliced.length === 1);
checks.push(sliced[0] === "sparse-val");

// Own-vs-prototype precedence: the new arr.GetOwn check must sit between
// the dense-element check and the prototype walk, not replace or bypass
// either. Uses a second, distinct index so mutating Array.prototype here
// can't affect the checks above. Both directions matter: an own property
// past maxDenseArrayDefineIndex must still shadow an identically-indexed
// inherited one (the new check could wrongly skip straight to the proto
// walk), and an index with no own property there must still fall through
// to find one on Array.prototype (the new check must not report a false
// negative that swallows the walk).
{
  const PROTO_IDX = 16777218; // maxDenseArrayDefineIndex + 2
  const key = String(PROTO_IDX);
  (Array.prototype as any)[key] = "proto-val";
  try {
    const own: any[] = [];
    Object.defineProperty(own, key, {
      value: "own-val",
      writable: true,
      enumerable: true,
      configurable: true,
    });
    checks.push(own.at(-1) === "own-val"); // own shadows inherited
    delete own[PROTO_IDX];
    checks.push(own.at(-1) === "proto-val"); // falls through to Array.prototype
  } finally {
    delete (Array.prototype as any)[key];
  }
}

checks.every((c) => c === true);

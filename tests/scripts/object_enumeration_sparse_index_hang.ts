// expect: true
// paserati#176/#178: a numeric array index too large to grow the dense
// .elements slice into (see maxDenseArrayDefineIndex, 2^24, in
// array_props.go) is tracked as a named property instead - but it still
// extends the array's `.length`, since a defineProperty call at index i
// always makes `.length` at least i+1 regardless of where the value is
// stored.
//
// Object.keys/values/entries/getOwnPropertyNames (object_init.go) and
// Reflect.ownKeys (reflect_init.go) all used to enumerate an array's own
// numeric-index properties by looping `for i := 0; i < arrObj.Length();
// i++` - correct for a normal array, but Length() reports `.length`, not
// how many real entries exist. An array holding three elements plus one
// property defined at index 4294967294 has a `.length` of 4294967295, so
// every one of those loops was a multi-billion-iteration hang instead of
// an O(1)-per-real-entry scan.
//
// Fixed by bounding the loop to DenseLength() (len of the actual .elements
// slice) and separately walking the array's tracked sparse/named entries
// (O(number of entries), not O(index value)) for whatever lies beyond it -
// this file asserts both that it completes quickly (the timing itself,
// implicitly - a regression back to the O(length) loop would make this
// whole script hang rather than report `false`) and that the huge index
// is still correctly visited, not silently dropped to avoid the hang.
const checks: boolean[] = [];

const huge: any[] = [1, 2, 3];
Object.defineProperty(huge, "4294967294", {
  value: "z",
  writable: true,
  enumerable: true,
  configurable: true,
});

checks.push(huge.length === 4294967295);

const keys = Object.keys(huge);
checks.push(keys.join(",") === "0,1,2,4294967294");

const values = Object.values(huge);
checks.push(JSON.stringify(values) === '[1,2,3,"z"]');

const entries = Object.entries(huge);
checks.push(
  JSON.stringify(entries) ===
    '[["0",1],["1",2],["2",3],["4294967294","z"]]'
);

const names = Object.getOwnPropertyNames(huge);
checks.push(names.join(",") === "0,1,2,4294967294,length");

const reflectKeys = Reflect.ownKeys(huge) as string[];
checks.push(reflectKeys.join(",") === "0,1,2,4294967294,length");

// Multiple sparse entries: must come out in ascending numeric order, not
// map-iteration order (which Go does not guarantee), and a named
// (non-index) property must sort after every index - both dense and
// sparse - per OrdinaryOwnPropertyKeys.
const multi: any[] = [10, 20];
Object.defineProperty(multi, "4294967290", {
  value: "second-highest",
  writable: true,
  enumerable: true,
  configurable: true,
});
Object.defineProperty(multi, "4294900000", {
  value: "lowest-sparse",
  writable: true,
  enumerable: true,
  configurable: true,
});
Object.defineProperty(multi, "4294967294", {
  value: "highest",
  writable: true,
  enumerable: true,
  configurable: true,
});
Object.defineProperty(multi, "named", {
  value: "not-an-index",
  writable: true,
  enumerable: true,
  configurable: true,
});
checks.push(
  Object.keys(multi).join(",") ===
    "0,1,4294900000,4294967290,4294967294,named"
);

// A non-enumerable sparse index must appear in getOwnPropertyNames/
// Reflect.ownKeys (which don't filter by enumerability) but not in
// Object.keys/values/entries/Object.assign (which do).
const withHidden: any[] = [];
Object.defineProperty(withHidden, "4294800000", {
  value: "hidden",
  writable: true,
  enumerable: false,
  configurable: true,
});
checks.push(Object.keys(withHidden).length === 0);
checks.push(Object.values(withHidden).length === 0);
checks.push(Object.entries(withHidden).length === 0);
checks.push(Object.getOwnPropertyNames(withHidden).join(",") === "4294800000,length");
checks.push((Reflect.ownKeys(withHidden) as string[]).join(",") === "4294800000,length");

// Object.assign only copies own enumerable properties, including a
// sparse index.
const assignSource: any[] = [1];
Object.defineProperty(assignSource, "4294967294", {
  value: "copied",
  writable: true,
  enumerable: true,
  configurable: true,
});
Object.defineProperty(assignSource, "4294800001", {
  value: "not-copied",
  writable: true,
  enumerable: false,
  configurable: true,
});
const assignTarget: any = Object.assign({}, assignSource);
checks.push(assignTarget["0"] === 1);
checks.push(assignTarget["4294967294"] === "copied");
checks.push(!("4294800001" in assignTarget));

// A sparse index defined as an ACCESSOR (get/set, not a plain value) is
// tracked in a completely different place than a sparse data property
// (ArrayObject's getters/setters maps, never its properties map - see
// DefineAccessorProperty) - and NOT just for indices past
// maxDenseArrayDefineIndex; an in-bounds index whose value never grew
// `elements` far enough to reach it (e.g. index 10000 on a 3-element
// array) is exactly as "sparse" here. Missing this case while fixing the
// hang regressed test262 built-ins/Object/keys/15.2.3.14-5-14.js during
// development of this fix, so it's asserted directly: the getter must be
// called (not silently dropped, and not read as a plain data value).
const accessorSparse: any[] = [1, 2, 3];
let getterCalls = 0;
Object.defineProperty(accessorSparse, "10000", {
  get() {
    getterCalls++;
    return "ElementWithLargeIndex";
  },
  enumerable: true,
  configurable: true,
});
checks.push(Object.keys(accessorSparse).join(",") === "0,1,2,10000");
checks.push(Object.values(accessorSparse).indexOf("ElementWithLargeIndex") === 3);
checks.push(getterCalls === 1);
checks.push(Object.assign({}, accessorSparse)["10000"] === "ElementWithLargeIndex");
checks.push(getterCalls === 2);

// The same, but at a genuinely huge (past maxDenseArrayDefineIndex)
// index, and enumerable:false so it must appear in getOwnPropertyNames/
// Reflect.ownKeys but not Object.keys/values/entries/assign.
const hugeAccessor: any[] = [];
Object.defineProperty(hugeAccessor, "4294967293", {
  get() {
    return "huge-accessor";
  },
  enumerable: false,
  configurable: true,
});
checks.push(Object.keys(hugeAccessor).length === 0);
checks.push(
  Object.getOwnPropertyNames(hugeAccessor).join(",") === "4294967293,length"
);
checks.push(
  (Reflect.ownKeys(hugeAccessor) as string[]).join(",") ===
    "4294967293,length"
);

// Side effect of bounding the dense loop to DenseLength(): an array whose
// `.length` exceeds DenseLength() with NOTHING tracked in the gap (no
// defineProperty call at all - just a length bump, or new Array(n)'s
// preallocated-but-empty slots) now correctly reports no own property for
// those indices, matching V8/Node (`Object.keys(new Array(5))` is `[]`,
// not `["0","1","2","3","4"]`; growing `.length` past an array's real
// elements doesn't create own properties either) - this was NOT true
// before this fix (Length() was the loop bound, so every one of those
// indices was incorrectly reported as an own key), but it's the same fix,
// not a separate behavior change: neither loop bound is spec's real
// answer for whether index i is an own property, and the sparse walk
// added alongside DenseLength() is what makes the new, narrower answer
// correct rather than merely convenient for the hang.
checks.push(Object.keys(new Array(5)).length === 0);
const growLength: any[] = [1, 2, 3];
growLength.length = 10;
checks.push(Object.keys(growLength).join(",") === "0,1,2");

// The for-in loop (pkg/vm/vm.go) walks an array's own keys through the
// exact same Length()-bounded scan objectKeysWithVM used to - same hang,
// same fix (DenseLength() + arraySparseIndices), applied there too since
// for-in is the most common way a user would actually hit this.
const forInHuge: any[] = [1, 2];
Object.defineProperty(forInHuge, "4294967290", {
  value: "sparse",
  writable: true,
  enumerable: true,
  configurable: true,
});
const forInKeys: string[] = [];
for (const k in forInHuge) forInKeys.push(k);
checks.push(forInKeys.join(",") === "0,1,4294967290");

checks.every((c) => c === true);

// expect: true
// paserati#174: objectAssignWithVM's copy loop had no branch at all for a
// vm.TypeArray target - only TypeObject/TypeDictObject - so Object.assign
// silently did nothing when the target was an array, as if the call had
// been a no-op. Distinct from paserati#168 (a wrong enumerable flag on
// object targets, already fixed); this is array targets never being
// handled in the first place.
const checks: boolean[] = [];

// The exact repro shape: tagging metadata onto a results array.
const arr = Object.assign([], { provisional: 0 });
checks.push(arr.provisional === 0);
arr.provisional++;
checks.push(arr.provisional === 1);

// Existing elements must survive, and a non-numeric key becomes a plain
// named property alongside them.
const arr2 = Object.assign([1, 2, 3], { extra: "hi" });
checks.push(JSON.stringify(arr2) === "[1,2,3]");
checks.push(arr2.extra === "hi");

// A numeric-string key becomes a real indexed element (extending the
// array, with holes for anything skipped), matching arr[idx] = value -
// not a named property literally called "0".
const arr3 = Object.assign([], { 0: "x", 3: "y" });
checks.push(arr3.length === 4);
checks.push(JSON.stringify(arr3) === '["x",null,null,"y"]');

// A huge numeric-index key (just under the 2^32-1 array length cap) must
// be stored as a sparse property, not by growing a dense elements slice -
// ArrayObject.Set is O(idx), so doing that naively hangs/OOMs on an index
// this large. Must resolve instantly, matching arr[idx] = value's own
// sparse-index guard (op_setprop.go / array_props.go). This asserts via
// length + hasOwnProperty rather than arr4[4294967294] itself: reading a
// defineProperty-stored sparse index below .length is a separate,
// pre-existing gap in the array bracket-read fast path (both op_getprop.go
// and OpGetIndex only ever consult .elements for a numeric index, never
// falling back to the named-property store), not something this fix
// touches or needs to fix.
const arr4 = Object.assign([], { 4294967294: "x" });
checks.push(arr4.length === 4294967295);
checks.push(arr4.hasOwnProperty("4294967294"));

checks.every((c) => c === true);

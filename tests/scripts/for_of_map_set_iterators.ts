// Exercises the Map/Set for-of fast path (internal iterator state on the
// iterator object behind the shared prototype next): live iteration with
// delete-during-iteration (tombstones skipped) and add-during-iteration
// (visited), keys/values/entries kinds, manual next() sharing state with
// for-of, and cross-brand next.call rejection.
// expect: m:a=1,b=2,c=3|del:a,c|add:1,2,3|keys:a,b|vals:1,2|s:x,y,z|sdel:x,z|se:y=y|mix:b=2|brand:ok
let out: any[] = [];

// Map entries via for-of (Symbol.iterator === entries)
const m = new Map<any, any>([["a", 1], ["b", 2], ["c", 3]]);
let acc: any[] = [];
for (const [k, v] of m) acc.push(k + "=" + v);
out.push("m:" + acc.join(","));

// Delete during iteration: "b" removed while visiting "a" -> skipped
const m2 = new Map<any, any>([["a", 1], ["b", 2], ["c", 3]]);
acc = [];
for (const [k, v] of m2) {
  acc.push(k);
  if (k === "a") m2.delete("b");
}
out.push("del:" + acc.join(","));

// Add during iteration: entries appended while iterating are visited
const m3 = new Map<any, any>([[1, "x"]]);
acc = [];
for (const [k, v] of m3) {
  acc.push(k);
  if (k < 3) m3.set(k + 1, "y");
}
out.push("add:" + acc.join(","));

// keys() and values()
const m4 = new Map<any, any>([["a", 1], ["b", 2]]);
acc = [];
for (const k of m4.keys()) acc.push(k);
out.push("keys:" + acc.join(","));
acc = [];
for (const v of m4.values()) acc.push(v);
out.push("vals:" + acc.join(","));

// Set values via for-of
const s = new Set<any>(["x", "y", "z"]);
acc = [];
for (const v of s) acc.push(v);
out.push("s:" + acc.join(","));

// Set delete during iteration
const s2 = new Set<any>(["x", "y", "z"]);
acc = [];
for (const v of s2) {
  acc.push(v);
  if (v === "x") s2.delete("y");
}
out.push("sdel:" + acc.join(","));

// Set entries: [v, v] pairs
const s3 = new Set<any>(["y"]);
acc = [];
for (const [a, b] of s3.entries()) acc.push(a + "=" + b);
out.push("se:" + acc.join(","));

// Manual next() shares state with for-of on a Map iterator
const mit: any = new Map<any, any>([["a", 1], ["b", 2]]).entries();
mit.next(); // consume ["a", 1]
acc = [];
for (const [k, v] of mit) acc.push(k + "=" + v);
out.push("mix:" + acc.join(","));

// Brand check: Map prototype next on a Set iterator must throw
const setIter: any = new Set<any>([1]).values();
const mapIter: any = new Map<any, any>().entries();
let branded = "no-throw";
try {
  mapIter.next.call(setIter);
} catch (e) {
  branded = "ok";
}
out.push("brand:" + branded);

out.join("|");

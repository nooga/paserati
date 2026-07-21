// Exercises Map/Set key identity under the allocation-free mapKey hashing:
// key-type disambiguation (string "1" != number 1, string "true" != boolean
// true), SameValueZero canonicalization (NaN === NaN, +0 === -0), object
// reference identity, and delete/re-insert insertion-order semantics.
// expect: 1|str1|T|strT|nan|z|z2|11|x,z,y|1,3,22|5|1,2,2,NaN,[object Object]
let out: string[] = [];

const m = new Map<any, any>();
m.set("a", 1); m.set(1, "n1"); m.set("1", "str1");
m.set(true, "T"); m.set("true", "strT");
m.set(null, "N"); m.set(undefined, "U");
const o1: any = {}, o2: any = {};
m.set(o1, "o1"); m.set(o2, "o2");
out.push(String(m.get("a")));       // 1  (string key)
out.push(m.get("1"));               // str1 (string "1" distinct from number 1)
out.push(m.get(true));              // T  (boolean true distinct from string "true")
out.push(m.get("true"));            // strT

m.set(NaN, "nan");
out.push(m.get(0 / 0));             // nan  (NaN === NaN)
m.set(0, "z");
out.push(m.get(-0));                // z    (+0 === -0)
m.set(-0, "z2");
out.push(m.get(0));                 // z2   (same key overwritten)

// size: a,1,"1",true,"true",null,undefined,o1,o2,NaN,0 = 11
out.push(String(m.size));           // 11

// delete + re-insert moves the key to the END of iteration order
const m2 = new Map<string, number>();
m2.set("x", 1); m2.set("y", 2); m2.set("z", 3);
m2.delete("y"); m2.set("y", 22);
out.push([...m2.keys()].join(","));   // x,z,y
out.push([...m2.values()].join(",")); // 1,3,22

// Set dedup across key types + reference identity
const s = new Set<any>([1, 2, 2, "2", NaN, NaN, o1, o1]);
out.push(String(s.size));           // 5
out.push([...s].join(","));         // 1,2,2,NaN,[object Object]  (o1 last)

out.join("|");

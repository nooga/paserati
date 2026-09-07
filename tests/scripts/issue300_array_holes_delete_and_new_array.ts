// expect: true
// paserati#300: array holes are materialized as present undefined slots -
// `in`, hasOwnProperty, Object.keys/getOwnPropertyNames/entries/values,
// Object.getOwnPropertyDescriptor, Reflect.ownKeys, Object.assign, and
// for-in all reported a hole (from `delete arr[i]` or `new Array(n)`) as a
// present own property instead of an absent one, because they checked only
// "is this index in bounds" instead of consulting the array's actual hole
// tracking (ArrayObject.HasIndex, already used correctly by
// hasOwnProperty/for-in/the iteration builtins before this fix - see the
// new ArrayObject.HasOwnIndexProperty helper that now unifies all of these).
//
// A hole created by a literal elision ([1,,3]) is a separate, NOT-yet-fixed
// gap (elisions are still materialized as real `undefined` values by the
// parser/compiler) - not covered here.
const checks: boolean[] = [];

// --- delete arr[i] ---
const d: any = [1, 2, 3];
delete d[1];
checks.push((1 in d) === false);
checks.push(d.hasOwnProperty(1) === false);
checks.push(Object.keys(d).join(",") === "0,2");
checks.push(Object.getOwnPropertyNames(d).join(",") === "0,2,length");
checks.push(JSON.stringify(Object.entries(d)) === '[["0",1],["2",3]]');
checks.push(Object.values(d).join(",") === "1,3");
checks.push(Object.getOwnPropertyDescriptor(d, "1") === undefined);
checks.push(Reflect.ownKeys(d).join(",") === "0,2,length");

const visitedForOf: number[] = [];
d.forEach((v: any, i: number) => visitedForOf.push(i));
checks.push(visitedForOf.join(",") === "0,2");

const visitedForIn: string[] = [];
for (const k in d) visitedForIn.push(k);
checks.push(visitedForIn.join(",") === "0,2");

const assigned: any = Object.assign({}, d);
checks.push(Object.keys(assigned).join(",") === "0,2");
checks.push(assigned[0] === 1 && assigned[2] === 3);

// --- new Array(n) ---
const s: any = new Array(3);
checks.push((0 in s) === false);
checks.push(s.hasOwnProperty(0) === false);
checks.push(Object.keys(s).join(",") === "");
checks.push(Object.getOwnPropertyDescriptor(s, "0") === undefined);
checks.push(Reflect.ownKeys(s).join(",") === "length");

// --- A defineProperty-tracked huge sparse index (paserati#176/#178) must
// still count as present through the same shared helper - a hole check
// must not start dropping these. (Object.keys/values/entries/
// getOwnPropertyNames/Reflect.ownKeys on such an array separately hang,
// pre-existing on main and unrelated to this fix - not exercised here;
// flagged as its own follow-up.) ---
const huge: any = [];
Object.defineProperty(huge, "4294967294", {
  value: "z",
  writable: true,
  enumerable: true,
  configurable: true,
});
checks.push(("4294967294" in huge) === true);
checks.push(huge.hasOwnProperty("4294967294") === true);
checks.push(
  Object.getOwnPropertyDescriptor(huge, "4294967294").value === "z"
);

checks.every((c) => c === true);

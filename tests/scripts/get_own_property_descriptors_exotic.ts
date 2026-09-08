// expect: true
// Object.getOwnPropertyDescriptors collected its keys via its own local
// switch on obj.Type() (separate from the single-key
// Object.getOwnPropertyDescriptor) - it had no case at all for
// TypeRegExp/TypeMap/TypeSet/TypePromise, so stringKeys/symbolKeys stayed
// empty and it always returned {} for one of these four kinds, even after
// a real own property had been defined or assigned on it, despite the
// single-key getOwnPropertyDescriptor already answering correctly for the
// exact same property (paserati#329).
const checks: boolean[] = [];

function hasDataDesc(
  descs: any,
  key: string,
  value: unknown,
  writable: boolean,
  enumerable: boolean,
  configurable: boolean
): boolean {
  const d = descs[key];
  return (
    d !== undefined &&
    d.value === value &&
    d.writable === writable &&
    d.enumerable === enumerable &&
    d.configurable === configurable
  );
}

// --- RegExp with no custom property still reports "lastIndex" (a real
// own property that lives on the RegExpObject itself, not the side
// table - the only kind of these four with an intrinsic own property to
// add alongside whatever the side table holds) ---
const descsEmpty: any = Object.getOwnPropertyDescriptors(/x/g);
checks.push(
  Object.keys(descsEmpty).length === 1 &&
    hasDataDesc(descsEmpty, "lastIndex", 0, true, false, false)
);

// --- RegExp with a custom property reports both ---
const r: any = /y/g;
r.custom = 1;
const descsR: any = Object.getOwnPropertyDescriptors(r);
checks.push(
  hasDataDesc(descsR, "lastIndex", 0, true, false, false) &&
    hasDataDesc(descsR, "custom", 1, true, true, true)
);

// --- Map/Set/Promise with a custom property ---
const m: any = new Map();
m.custom = 2;
checks.push(hasDataDesc(Object.getOwnPropertyDescriptors(m), "custom", 2, true, true, true));

const s: any = new Set();
s.custom = 3;
checks.push(hasDataDesc(Object.getOwnPropertyDescriptors(s), "custom", 3, true, true, true));

const p: any = Promise.resolve(1);
p.custom = 4;
checks.push(hasDataDesc(Object.getOwnPropertyDescriptors(p), "custom", 4, true, true, true));

// --- a symbol key round-trips too (via Object.defineProperty - bracket
// assignment with a computed key on these kinds has its own, separate,
// unrelated pre-existing gap) ---
const m2: any = new Map();
const sym = Symbol("k");
Object.defineProperty(m2, sym, {
  value: 5,
  writable: true,
  enumerable: true,
  configurable: true,
});
const descsSym: any = Object.getOwnPropertyDescriptors(m2);
checks.push(
  descsSym[sym] !== undefined &&
    descsSym[sym].value === 5 &&
    descsSym[sym].writable &&
    descsSym[sym].enumerable &&
    descsSym[sym].configurable
);

// --- a Map/Set/Promise with nothing custom on it reports no custom keys
// (Map/Set's own "size" isn't an own property, it's an accessor on
// Map.prototype/Set.prototype). Object.keys on the *result* object counts
// string keys only - fine here since none of these three fixtures carry
// a symbol; the m2 case above is what covers symbol-key round-tripping. ---
checks.push(Object.keys(Object.getOwnPropertyDescriptors(new Map())).length === 0);
checks.push(Object.keys(Object.getOwnPropertyDescriptors(new Set())).length === 0);
checks.push(Object.keys(Object.getOwnPropertyDescriptors(Promise.resolve(1))).length === 0);

checks.every((c) => c === true);

// Exercises the array-destructuring fast path (OpArrayDestructFastCheck /
// OpArrayRawGetInt): pristine plain arrays are destructured by direct index
// read with no iterator object, next() call, or {value, done} allocation.
// Covers the behaviors it must preserve: short source -> undefined tail,
// holes -> undefined, defaults, elisions, nested patterns, rest (which stays on
// the generic path), for-of element patterns, and deopt to the generic protocol
// when Symbol.iterator is customized on the prototype.
// expect: 1,2|10,undefined,undefined|5,20|1,undefined,3|2,4|7,8,9|1;2,3,4|a=1,b=2|100,200
let out: string[] = [];

// basic + fewer-than-pattern tail
const [a, b] = [1, 2];
const [c, d, e] = [10] as number[];
out.push([a, b].join(","));
out.push([c, d, e].join(","));

// defaults + holes
const [f = 5, g = 6] = [undefined, 20];
const arr: any[] = [1, , 3];
const [h, i, j] = arr;
out.push([f, g].join(","));
out.push([h, i, j].join(","));

// elision + nested
const [, k, , l] = [1, 2, 3, 4];
const [[m, n], o] = [[7, 8], 9];
out.push([k, l].join(","));
out.push([m, n, o].join(","));

// rest (generic path) + for-of element pattern over a Map
const [p, ...rest] = [1, 2, 3, 4];
out.push(p + ";" + rest.join(","));
const mp = new Map([["a", 1], ["b", 2]]);
let mparts: string[] = [];
for (const [mk, mv] of mp) mparts.push(mk + "=" + mv);
out.push(mparts.join(","));

// prototype-level Symbol.iterator override must deopt to the generic protocol
const origIt = Array.prototype[Symbol.iterator];
(Array.prototype as any)[Symbol.iterator] = function () {
  let idx = 0;
  const self: any = this;
  return {
    next: () =>
      idx < self.length
        ? { value: self[idx++] * 100, done: false }
        : { value: undefined, done: true },
  };
};
const [q, r] = [1, 2];
(Array.prototype as any)[Symbol.iterator] = origIt;
out.push([q, r].join(","));

out.join("|");

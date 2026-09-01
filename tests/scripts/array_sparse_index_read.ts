// expect: true
// paserati#176: when a numeric array index is stored as a named property
// via ArrayObject.DefineOwnProperty rather than in the dense .elements
// slice - because the index is too large to grow .elements into (see
// maxDenseArraySetIndex/maxDenseArrayDefineIndex, 2^24) - reading it back
// via arr[idx] or arr["idx"] used to always return undefined: the
// bracket-read fast paths (op_getprop.go, OpGetIndex, and the Proxy
// array-target fallback in vm.go, plus vm.GetProperty in vm_init.go) all
// short-circuited to Undefined whenever the index fell inside .length but
// outside .elements, without ever consulting the named-property store the
// value actually lives in. hasOwnProperty/getOwnPropertyDescriptor always
// saw the property correctly - only the bracket read was broken.
const checks: boolean[] = [];

const a: any[] = [];
Object.defineProperty(a, "4294967294", {
  value: "z",
  writable: true,
  enumerable: true,
  configurable: true,
});
checks.push(a.length === 4294967295);
checks.push(a[4294967294] === "z"); // numeric-literal index read
checks.push(a["4294967294"] === "z"); // string-key index read
checks.push((a as any).hasOwnProperty("4294967294"));

// A genuine sparse hole (no property ever defined at that index) must
// still read back as undefined - this fix must not turn every gap into a
// property-store probe that finds something that isn't there.
const b: any[] = [];
b[5] = "only-element";
checks.push(b[2] === undefined);
checks.push(b.length === 6);

// Companion fix: ArraySetLength (10.4.2.4 step 3.l) must also delete an
// array-index property stored in .properties once it truncates below the
// new length, not just dense .elements - this was dead code before the
// read-path fix above (such an index always read back as undefined
// anyway, so nothing could tell the property had survived the
// truncation). Regressed test262's built-ins/Array/length/S15.4.5.2_A3_T4
// until fixed alongside the read path.
const d: any[] = [0, 1, 2];
Object.defineProperty(d, "4294967294", {
  value: "gone",
  writable: true,
  enumerable: true,
  configurable: true,
});
d.length = 2;
checks.push(d[0] === 0 && d[1] === 1);
checks.push(d[4294967294] === undefined);
checks.push(!(d as any).hasOwnProperty("4294967294"));

checks.every((c) => c === true);

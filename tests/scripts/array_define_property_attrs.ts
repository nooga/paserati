// expect: true
// paserati#178: Object.defineProperty on an array-index property silently
// ignored the requested writable/enumerable/configurable attributes and
// always stored {writable:true, enumerable:true, configurable:true} -
// both for a dense in-range index (backed by ArrayObject's .elements
// slice) and a huge sparse index (beyond maxDenseArrayDefineIndex /
// maxDenseArraySetIndex, 2^24, tracked in the named-property map
// instead) - unlike the identical call on a plain object, which already
// honored the flags and defaulted unspecified ones to false.
const checks: boolean[] = [];

// Dense index: explicit configurable:false must be honored.
const a: any[] = [];
Object.defineProperty(a, "2", { value: "v", configurable: false });
const descA = Object.getOwnPropertyDescriptor(a, "2")!;
checks.push(descA.value === "v");
checks.push(descA.writable === false); // unspecified -> defaults to false
checks.push(descA.enumerable === false); // unspecified -> defaults to false
checks.push(descA.configurable === false);

// Redefining a non-configurable dense index must throw.
let threw = false;
try {
  Object.defineProperty(a, "2", { configurable: true });
} catch (e) {
  threw = e instanceof TypeError;
}
checks.push(threw);

// Deleting a non-configurable dense index must fail (sloppy mode:
// silently - the property must still be there afterward).
delete a[2];
checks.push((a as any).hasOwnProperty("2"));
checks.push(a[2] === "v");

// A dense index explicitly defined with every attribute true must report
// exactly the ES default - no accidental non-default leftover.
const b: any[] = [];
Object.defineProperty(b, "5", {
  value: "w",
  writable: true,
  enumerable: true,
  configurable: true,
});
const descB = Object.getOwnPropertyDescriptor(b, "5")!;
checks.push(
  descB.writable === true &&
    descB.enumerable === true &&
    descB.configurable === true,
);
delete b[5]; // configurable: true -> must succeed
checks.push(!(b as any).hasOwnProperty("5"));

// A plain arr[i] = v assignment (never through defineProperty) must still
// report the ES default - this fix must not change that common path.
const c: any[] = [1, 2, 3];
const descC = Object.getOwnPropertyDescriptor(c, "1")!;
checks.push(
  descC.writable === true &&
    descC.enumerable === true &&
    descC.configurable === true,
);

// Huge sparse index (beyond the dense-allocation bound): same attribute
// handling as the dense case above.
const d: any[] = [];
Object.defineProperty(d, "4294967290", { value: "z", configurable: false });
const descD = Object.getOwnPropertyDescriptor(d, "4294967290")!;
checks.push(descD.value === "z");
checks.push(
  descD.writable === false &&
    descD.enumerable === false &&
    descD.configurable === false,
);
delete d[4294967290];
checks.push((d as any).hasOwnProperty("4294967290")); // survives: non-configurable

// Object.freeze must still win over an earlier defineProperty-tracked
// writable:true/configurable:true on a dense index - freeze can only take
// capabilities away, never leave one an explicit defineProperty granted.
// This also exercises Object.isFrozen(), which re-validates by walking
// every tracked descriptor: a stale writable/configurable:true entry left
// behind by freeze would make isFrozen() wrongly report false even though
// Object.freeze just ran.
const e: any[] = [];
Object.defineProperty(e, "2", {
  value: "v",
  writable: true,
  enumerable: false,
  configurable: true,
});
Object.freeze(e);
checks.push(Object.isFrozen(e));
const descE = Object.getOwnPropertyDescriptor(e, "2")!;
checks.push(descE.writable === false && descE.configurable === false);
delete e[2];
checks.push((e as any).hasOwnProperty("2")); // survives: freeze wins

checks.every((c) => c === true);

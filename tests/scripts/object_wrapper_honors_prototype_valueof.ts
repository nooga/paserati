// expect: 2:2:true:true
// Object(primitive) must produce a wrapper whose only own property is the
// internal primitive slot, exactly like `new Number(x)` already did. It used to
// install its own valueOf returning the boxed primitive, which shadowed the
// prototype's - so a user-overridden Number.prototype.valueOf / BigInt.prototype
// .valueOf was never consulted for a wrapper built by Object(), and ToPrimitive
// silently used the shadowing copy instead (#115).
let valueOfGets = 0;
let valueOfCalls = 0;

const NumberValueOf = Number.prototype.valueOf;
Object.defineProperty(Number.prototype, "valueOf", {
  get: () => {
    valueOfGets++;
    return function (this: any) {
      valueOfCalls++;
      return NumberValueOf.call(this) * 2;
    };
  },
  configurable: true,
});

const eqUsesOverride = Object(1) == 2;
const addUsesOverride = Object(3) + 0 === 6;

// Own properties must match what `new Number(1)` has - no shadowing valueOf.
const wrapperOwn = Object.getOwnPropertyNames(Object(1)).join(",");
const constructedOwn = Object.getOwnPropertyNames(new Number(1)).join(",");

valueOfGets + ":" + valueOfCalls + ":" + eqUsesOverride + ":" +
  (addUsesOverride && wrapperOwn === constructedOwn);

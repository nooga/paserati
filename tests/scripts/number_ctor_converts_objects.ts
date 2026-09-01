// expect: 7:5:0:0:7:NaN:true
// Number(x) has to convert an object argument the way the spec does -
// ToNumber(ToPrimitive(x, "number")). It used to reach into a wrapper's
// [[PrimitiveValue]] slot and fall through to NaN for every other object, so
// Number(obj) never converted an ordinary object at all, while unary + on the
// same value was correct because that path goes through the VM's ToPrimitive.
const viaValueOf = Number({ valueOf: () => 7 } as any);
const viaArray = Number([5] as any);
const viaEmptyArray = Number([] as any);
const viaDate = Number(new Date(0) as any);
const viaStringWrapper = Number(Object("7") as any);
const plainObject = Number({} as any);

// Both must agree, and both must go through the prototype method.
const agreesWithUnaryPlus = Number({ valueOf: () => 7 } as any) === +({ valueOf: () => 7 } as any);

viaValueOf + ":" + viaArray + ":" + viaEmptyArray + ":" + viaDate + ":" +
  viaStringWrapper + ":" + plainObject + ":" + agreesWithUnaryPlus;

// BigInt.prototype methods and the BigInt() constructor must accept a primitive
// BigInt receiver. Regression: each site guarded its object-wrapper unwrap with
// `Type() == TypeBigInt` and then called AsPlainObject(), which panics unless
// the value is TypeObject. A wrapper is TypeObject and never TypeBigInt, so the
// wrapper branch was unreachable and every primitive receiver panicked — the
// process exited 0 with no output and no error.
//
// Method calls go through `any` because the checker does not yet support
// property access on a bigint receiver at all ("property access is not
// supported on type bigint"), and it rejects unary '-' on bigint. Both are
// pre-existing checker gaps, separate from the runtime bug covered here.
// expect: 10|a|-ff|bigint|10|10|bigint|10|10|10|10n|10n|str|num
let out: string[] = [];
const b: any = 10n;
const neg: any = BigInt("-255");
const wrapper: any = Object(10n);

// toString, including a radix and a negative value
out.push(b.toString());
out.push(b.toString(16));
out.push(neg.toString(16));

// valueOf returns the primitive BigInt, not a wrapper
out.push(typeof b.valueOf());
out.push(String(b.valueOf()));

// toLocaleString
out.push(b.toLocaleString());

// BigInt() applied to something that is already a BigInt
out.push(typeof BigInt(10n));
out.push(String(BigInt(10n)));

// An object wrapper receiver unwraps to its primitive. This is the arm the
// helper exists for, and it only works if the slot name matches the one
// Object(bigint) actually writes.
out.push(wrapper.toString());
out.push(wrapper.toLocaleString());
out.push(String((BigInt.prototype as any).valueOf.call(wrapper)) + "n");
out.push(String(BigInt(wrapper)) + "n");

// Non-BigInt receivers keep their existing fallbacks: toString stringifies,
// valueOf throws.
out.push((BigInt.prototype as any).toString.call("str"));
try {
  (BigInt.prototype as any).valueOf.call(5);
  out.push("no-throw");
} catch (e) {
  out.push("num");
}

out.join("|");

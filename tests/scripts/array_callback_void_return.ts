// Regression test: a callback argument whose inferred return type is `void`
// (i.e. it has no return statement) must still be assignable wherever the
// callback parameter's function type declares a specific return type,
// matching TypeScript's special-casing of void-returning callbacks. Before
// this fix, forEach's callback parameter type is `(value, index?, array?) =>
// undefined`, and a plain arrow function with an implicit void return (no
// `return` statement) was rejected with:
//   Argument of type '(any, any) => void' is not assignable to parameter of
//   type '(any, number?, any[]?) => undefined'.
// expect: 21

const values: any[] = [1, 2, 3, 4, 5, 6];

let sum = 0;

// forEach's callback parameter type declares an `undefined` return type, but
// this callback implicitly returns void (no return statement at all).
values.forEach((v, i) => {
	sum += v;
	console.log("forEach", i, v);
});

// Same, but the callback only uses a subset of the optional parameters.
values.forEach((v) => {
	console.log("forEach-one-arg", v);
});

// Same, with zero parameters.
values.forEach(() => {
	console.log("forEach-no-args");
});

// map/filter/some/every callbacks that DO return a value should keep working
// (regression coverage for the normal, non-void case).
const doubled = values.map((v) => v * 2);
const evens = values.filter((v) => v % 2 === 0);
const hasBig = values.some((v) => v > 5);
const allPositive = values.every((v) => v > 0);

if (doubled.length !== values.length) {
	throw new Error("map result length mismatch");
}
if (evens.length !== 3) {
	throw new Error("filter result mismatch");
}
if (!hasBig) {
	throw new Error("some result mismatch");
}
if (!allPositive) {
	throw new Error("every result mismatch");
}

// sort's comparefn parameter is optional, so its declared type is a union
// `((a, b) => number) | undefined` — the void-tolerant check must unwrap
// that union to reach the callable member.
let sortCallbackRan = false;
values.sort((a) => {
	sortCallbackRan = true;
	console.log("sort", a);
});
if (!sortCallbackRan) {
	throw new Error("sort comparefn was never called");
}

sum;

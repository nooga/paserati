// expect_compile_error: not assignable

// The void-returning-callback special case only relaxes the RETURN type of a
// callback argument; parameter types must still be checked normally, and a
// non-callback assignment of a void function should still be rejected.
const values: number[] = [1, 2, 3];

// Wrong parameter type (string instead of number) - must still error even
// though the callback body has no return statement.
values.forEach((v: string) => {
	console.log(v);
});

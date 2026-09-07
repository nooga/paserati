// expect: true
// Array.prototype.join() called with zero arguments (relying on the
// default "," separator) produced the wrong output when TypeScript type
// checking is enabled (the default mode this test runs in - no
// // no-typecheck directive - specifically to catch this): the compiler
// pads a call site to a native function's checker-known arity with
// explicit `undefined` arguments for any parameter the checker considers
// optional-but-unprovided (compileArgumentsWithOptionalHandling,
// pkg/compiler/compiler.go), so `arr.join()` arrived at the native `join`
// implementation as `args = [undefined]` rather than `args = []`. The
// implementation's `len(args) >= 1` check then treated that padded
// argument as an explicitly-provided separator and stringified it,
// joining elements with the literal string "undefined" instead of ",".
//
// Per ECMA-262 23.1.3.16 step 3, an explicit `undefined` separator must
// ALSO default to "," - the fix checks the value itself, not just whether
// an argument slot was filled, which handles both the type-checker padding
// and a real `arr.join(undefined)` call the same (correct) way.
const checks: boolean[] = [];

checks.push([1, 2, 3].join() === "1,2,3");
checks.push([1, 2, 3].join(undefined) === "1,2,3");
checks.push([1, 2, 3].join(",") === "1,2,3");
checks.push([1, 2, 3].join("-") === "1-2-3");
checks.push(Object.keys([1, 2, 3]).join() === "0,1,2");
checks.push(Object.keys({ a: 1, b: 2 }).join() === "a,b");
checks.push([].join() === "");
checks.push(["only"].join() === "only");

checks.every((c) => c === true);

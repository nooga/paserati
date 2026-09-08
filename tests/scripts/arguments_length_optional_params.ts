// expect: true
// `arguments.length` must equal the number of arguments the caller actually
// wrote - never inflated by the callee's own declared optional/default
// parameters. Under type checking (the default `./paserati script.ts`
// mode), a call omitting a value for a trailing optional parameter used to
// see arguments.length one higher than it should, because the compiler
// (determineTotalArgCount / compileArgumentsWithOptionalHandling in
// pkg/compiler, and their `...ForNew` counterparts for `new` expressions)
// padded the call site's argument count up to the callee's declared
// parameter count and emitted an explicit OpLoadUndefined into the gap:
//
//   function f(a?: string) { return arguments.length; }
//   f()   // was 1 - should be 0
//
// That padding turned out to be entirely redundant for its actual purpose
// (making a default parameter value apply when its argument is omitted):
// the VM's own call setup (prepareCall in pkg/vm/call.go, OpNew's
// constructor-frame setup, and OpTailCall/OpTailCallMethod's frame-reuse
// path) already independently fills every declared parameter register from
// the real argument count up to the callee's Arity with Undefined,
// regardless of what count the call site passes - which is exactly why
// default values already worked correctly under --no-typecheck, where the
// compiler-side padding never ran (no static function type to consult
// there). Removing the padding made the two modes agree.
const checks: boolean[] = [];

// --- Plain function, optional parameter, explicit vs. omitted argument ---
function optionalFn(a?: string): number {
  return arguments.length;
}
checks.push(optionalFn() === 0);
checks.push(optionalFn(undefined) === 1);
checks.push(optionalFn("x") === 1);

// --- Plain function, default-valued parameter: value still applies AND
// arguments.length is still the real count. ---
function defaultFn(a: number = 5): number {
  return a * 1000 + arguments.length;
}
checks.push(defaultFn() === 5000); // default applied, 0 args
checks.push(defaultFn(undefined) === 5001); // default applied even though "passed", 1 arg
checks.push(defaultFn(10) === 10001); // explicit value wins, 1 arg

// --- Multiple optional parameters, only some omitted. ---
function multiOptional(a?: string, b?: string, c?: string): number {
  return arguments.length;
}
checks.push(multiOptional() === 0);
checks.push(multiOptional("a") === 1);
checks.push(multiOptional("a", "b") === 2);
checks.push(multiOptional("a", "b", "c") === 3);

// --- Rest parameter after an optional parameter: neither arguments.length
// nor the rest array's contents are affected by the omitted optional arg. ---
function withRest(a?: string, ...rest: number[]): string {
  return arguments.length + ":" + JSON.stringify(rest);
}
checks.push(withRest() === "0:[]");
checks.push(withRest("x") === "1:[]");
checks.push(withRest("x", 1, 2) === "3:[1,2]");

// --- arguments object's actual contents, not just its length. ---
function argsContents(a?: string): string {
  return JSON.stringify(Array.from(arguments));
}
checks.push(argsContents() === "[]");
checks.push(argsContents("x") === '["x"]');

// --- Method calls. ---
class Calc {
  op(a?: number): number {
    return arguments.length;
  }
}
const calc = new Calc();
checks.push(calc.op() === 0);
checks.push(calc.op(undefined) === 1);
checks.push(calc.op(5) === 1);

// --- Constructor calls (`new`), including via super(). ---
class Base {
  len: number = -1;
  constructor(a?: string) {
    this.len = arguments.length;
  }
}
checks.push(new Base().len === 0);
checks.push(new Base(undefined).len === 1);
checks.push(new Base("x").len === 1);

class Derived extends Base {
  constructor() {
    super();
  }
}
checks.push(new Derived().len === 0);

// --- Overloaded function declarations still dispatch/compile correctly. ---
function overloaded(a: string): string;
function overloaded(a: string, b: number): string;
function overloaded(a: string, b?: number): string {
  return arguments.length + ":" + a + ":" + b;
}
checks.push(overloaded("x") === "1:x:undefined");
checks.push(overloaded("x", 5) === "2:x:5");

// --- Native builtins that rely on len(args) to detect an omitted optional
// argument must see the real count too (the bug's original symptom). ---
checks.push([1, 2, 3].join() === "1,2,3");
checks.push([1, 2, 3, 4].slice().length === 4);
checks.push("5".padStart(3) === "  5");

checks.every((c) => c === true);

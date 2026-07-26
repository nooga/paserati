// Typing rules for &&, || and ??: each unions the retained half of its left
// operand (falsy for &&, truthy for ||, non-nullish for ??) with the right
// operand's type, short-circuiting to a single side when the left operand
// alone decides the result.

let flag: boolean = true;
let maybeNum: number | undefined = 5;

// && retains the falsy half of the left operand.
let a: number | false = flag && 42;

// || retains the truthy half, with subtype reduction (number | undefined
// reduces to plain number here, since undefined can't survive ||).
let b: number = maybeNum || 0;

// ?? retains everything but null/undefined.
let c: number = maybeNum ?? 0;

// Contextual typing flows through && to the right operand. `1` is always
// truthy, so the && short-circuits entirely to the right operand, and the
// arrow's parameter is inferred as string rather than implicit any.
function take(f: (s: string) => number): number {
  return f("hi");
}
let viaAnd = take(1 && ((s) => s.length));

// Object spread distributes over the union a chained && produces, treating
// a falsy branch as contributing no properties.
let cnd1: boolean = true;
let cnd2: boolean = true;
const merged = {
  ...(cnd1 && { x: 1 }),
  ...(cnd2 && { y: 2 }),
};

// A literal type that survives a || (or ??) union via subtype reduction is
// not "fresh" and doesn't widen when inferred into a `let` without an
// annotation.
let lit: "foo" = "foo";
let widened = lit || "foo";
let stillFoo: "foo" = widened;

// expect: true
a === 42 &&
  b === 5 &&
  c === 5 &&
  viaAnd === 2 &&
  merged.x === 1 &&
  merged.y === 2 &&
  stillFoo === "foo";

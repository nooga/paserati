// expect: true
// paserati#276: a destructuring-assignment target that resolves to a
// variable captured (by reference) from an ENCLOSING function was compiled
// as if it were a plain local register of the CURRENT function -
// compileDestructuringTargetRef/assignToDestructuringTargetRef's identifier
// case (and its siblings for `for (x of ...)` reassignment and rest-element
// targets) only checked IsGlobal/IsSpilled, never whether the resolved
// symbol actually belonged to this function's own register space. Writing
// straight to that foreign register number either silently clobbered
// whatever unrelated local of the CURRENT function happened to share that
// number, or - if the number exceeded the current function's own
// RegisterSize - crashed the VM outright with a raw
// "index out of range [N] with length M" panic, no JS-visible error at all.
//
// This is #276's actual root cause: the panic's own call stack (three
// levels of nested generator resumption, from Babel/gensync's config
// loader) was purely incidental context around a call to an ordinary,
// non-generator closure - not itself a generator/async bug. The minimal
// repro below is extracted from the exact real-world function that
// panicked (@babel/generator's comment-stripping helper, structurally
// `list.forEach(node => { [node.a, out] = f(node.a, out); ... })`).
const checks: boolean[] = [];

// --- Case 1: plain identifier destructuring target from an enclosing scope ---
function makeAccumulator(): (list: { v: number }[]) => number {
  let total = 0;
  function step(t: { v: number }): undefined {
    // `total` is captured from `makeAccumulator`'s own frame, not a local of
    // `step`. The array literal on the left mixes a member-expression target
    // (t.v, local) with a plain-identifier target (total, captured) - the
    // exact shape that panicked.
    [t.v, total] = [t.v * 2, total + t.v];
  }
  return (list) => {
    list.forEach(step);
    return total;
  };
}
const acc = makeAccumulator();
checks.push(acc([{ v: 1 }, { v: 2 }, { v: 3 }]) === 6); // 1 + 2 + 3

// --- Case 2: `for (capturedVar of items)` reassigning an enclosing local ---
function makeLast(): (items: number[]) => number | undefined {
  let last: number | undefined;
  function run(items: number[]): void {
    for (last of items) {
      // body intentionally empty - just exercises the reassignment path
    }
  }
  return (items) => {
    run(items);
    return last;
  };
}
const lastOf = makeLast();
checks.push(lastOf([10, 20, 30]) === 30);

// --- Case 3: rest-element destructuring target from an enclosing scope ---
function makeRestCollector(): (arr: number[]) => number[] | undefined {
  let rest: number[] | undefined;
  function run(arr: number[]): void {
    let first: number;
    [first, ...rest] = arr;
  }
  return (arr) => {
    run(arr);
    return rest;
  };
}
const restOf = makeRestCollector();
const restResult = restOf([1, 2, 3, 4]);
checks.push(
  restResult !== undefined &&
    restResult.length === 3 &&
    restResult[0] === 2
);

// --- Case 4: a default-value expression that itself reads the captured
// target (exercises compileConditionalAssignmentWithTargetRef's read of
// ref.Symbol through the same upvalue path the write above uses) ---
function makeDefaultReader(): (list: { v: number }[]) => number {
  let total = 0;
  function step(t: { v: number }): undefined {
    [t.v, total = total + 1] = [t.v * 2, undefined];
  }
  return (list) => {
    list.forEach(step);
    return total;
  };
}
const defaultReader = makeDefaultReader();
checks.push(defaultReader([{ v: 1 }, { v: 2 }]) === 2);

// --- Case 5: the same captured variable read AND written within one
// destructuring assignment (checks addFreeSymbol's dedup against whatever
// index a separate read of the same upvalue already registered) ---
function makeReadWriteSame(): number {
  let r = 1;
  function inner(a: { x: number }): number {
    [a.x, r] = [r, r + 1];
    return r;
  }
  return inner({ x: 0 });
}
checks.push(makeReadWriteSame() === 2);

checks.every((c) => c);

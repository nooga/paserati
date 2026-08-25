// `for (using x of xs)` must check disposability per iteration BEFORE running
// the loop body - per spec the dispose method is looked up when the
// declaration is evaluated, not when disposal runs. A non-disposable value
// must throw TypeError("Object is not disposable.") before any side effects
// from that iteration's body, matching what plain `using` already does.
let log: string[] = [];
function res(name: string) {
  return {
    [Symbol.dispose]() {
      log.push("dispose:" + name);
    },
  };
}

// Valid case: disposal happens once per iteration, in order.
for (using r of [res("a"), res("b")]) {
  log.push("body");
}

// Invalid case: a non-disposable value throws before the body runs.
let threw = false;
let sawBody = false;
try {
  for (using x of [1, 2, 3]) {
    sawBody = true;
  }
} catch (e) {
  threw = e instanceof TypeError;
}

(log.join(",") === "body,dispose:a,body,dispose:b") && threw && !sawBody;
// expect: true

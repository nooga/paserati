// expect: boom|boom|boom|boom|[object Object]|TypeError
// A throwing toString/valueOf accessor must propagate its own exception out of
// every string-coercion path, instead of being masked by a second
// "Cannot convert object to primitive value" TypeError that escaped try/catch.

function caught(fn: () => void): string {
  try {
    fn();
    return "no-throw";
  } catch (e) {
    return e.message;
  }
}

// The `return` after each `throw` is unreachable on purpose. Paserati's checker
// raises TS2378 ("A 'get' accessor must return a value") for a getter body that
// only throws, which real TypeScript does not. Removing these makes the file
// stop compiling; drop them once TS2378 accounts for throw-only bodies.
const throwingToString = {
  get toString() {
    throw new Error("boom");
    return "unreachable";
  },
};

const throwingValueOf = {
  get valueOf() {
    throw new Error("boom");
    return 0;
  },
  toString() {
    return "unused";
  },
};

const results: string[] = [];

// Template literal substitution (OpStringConcat with a "string" hint).
results.push(caught(() => { `${throwingToString}`; }));
// Explicit ToString.
results.push(caught(() => { String(throwingToString); }));
// String concatenation via OpAdd.
results.push(caught(() => { "" + throwingToString; }));
// Numeric coercion prefers valueOf, so its getter throws first.
results.push(caught(() => { +throwingValueOf; }));

// A non-throwing object still stringifies normally inside a template.
const plain = { a: 1 };
results.push(`${plain}`);

// An object with no usable primitive conversion still reports the real
// TypeError rather than a stale/incorrect exception.
const unconvertible = { valueOf: null, toString: null };
try {
  `${unconvertible}`;
  results.push("no-throw");
} catch (e) {
  results.push(e.name);
}

results.join("|");

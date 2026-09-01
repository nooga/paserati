// expect: true
// paserati#168: Object.assign copied properties onto a PlainObject target as
// non-enumerable (SetOwnNonEnumerable instead of SetOwn), making them
// invisible to Object.keys/for-in/JSON.stringify/spread and to a second
// Object.assign call reading the first result as its source.
const merged = Object.assign({}, { a: 1 });

const checks: boolean[] = [];
checks.push(merged.a === 1);
checks.push(JSON.stringify(Object.keys(merged)) === '["a"]');
checks.push(JSON.stringify(merged) === '{"a":1}');
checks.push(JSON.stringify({ ...merged }) === '{"a":1}');

let seenInForIn = false;
for (const k in merged) {
  if (k === "a") seenInForIn = true;
}
checks.push(seenInForIn);

const desc = Object.getOwnPropertyDescriptor(merged, "a");
checks.push(desc!.enumerable === true);

// A property already on the target before the copy must stay enumerable too.
const o = Object.assign({ b: 2 }, { a: 1 });
checks.push(JSON.stringify(Object.keys(o)) === '["b","a"]');

// A value copied by one Object.assign must survive being the *source* of a
// second Object.assign (this is what compounds the bug in practice).
const step1 = Object.assign({}, { fn: () => "hi" });
const step2 = Object.assign({}, step1);
checks.push(typeof step2.fn === "function");
checks.push(JSON.stringify(Object.keys(step2)) === '["fn"]');

checks.every((c) => c === true);

// expect: true
// paserati#254: Object.assign onto a function target (plain, closure, or
// Function.prototype.bind() result) matched none of setObjectAssignTargetProperty's
// dispatch cases and silently dropped every source property - the exact
// construction pattern @babel/template's public API uses (bind the callable,
// then Object.assign named sub-builders onto it).
const checks: boolean[] = [];

function plain() {}
Object.assign(plain as any, { a: 1 });
checks.push((plain as any).a === 1);
checks.push(plain.hasOwnProperty("a"));

function base() {}
const bound = base.bind(undefined);
Object.assign(bound as any, { a: 1 });
checks.push((bound as any).a === 1);
checks.push(bound.hasOwnProperty("a"));

let y = 10;
const closureFn = () => y;
Object.assign(closureFn as any, { c: 3 });
checks.push((closureFn as any).c === 3);
checks.push(closureFn.hasOwnProperty("c"));

// Also verify Object.keys/values/entries see it, not just direct reads.
Object.assign(bound as any, { b: 2 });
checks.push(JSON.stringify(Object.keys(bound)) === '["a","b"]');
checks.push(JSON.stringify(Object.values(bound)) === "[1,2]");
checks.push(JSON.stringify(Object.entries(bound)) === '[["a",1],["b",2]]');

// The actual reported scenario: @babel/template's public API binds a
// callable and Object.assign's named sub-builders onto it, then calls one
// of those sub-builders as a method (`n.template.statement(...)`).
function tplBase() {}
const tpl = Object.assign(tplBase.bind(undefined) as any, {
  statement: (s: string) => "stmt:" + s,
  ast: 1,
});
checks.push(tpl.statement("x") === "stmt:x");
checks.push(tpl.ast === 1);

checks.every((c) => c === true);

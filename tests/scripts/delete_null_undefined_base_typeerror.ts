// expect: TypeError,TypeError,TypeError,TypeError
// Per ECMAScript delete-operator semantics, `delete base.prop` / `delete
// base[key]` fail ToObject(base) with a TypeError when base is null or
// undefined - not a ReferenceError (test262
// language/expressions/delete/member-{identifier,computed}-reference-
// {null,undefined}.js). This test262 regression was masked by issue #65
// until the throw-site fixes let assert.throws' own mismatch-detection
// throw actually surface.
let results: string[] = [];

function nameOf(e: unknown): string {
  return e instanceof TypeError ? "TypeError" : e instanceof ReferenceError ? "ReferenceError" : "other";
}

try {
  let base: any = null;
  delete base.prop;
  results.push("no throw");
} catch (e) {
  results.push(nameOf(e));
}

try {
  let base: any = undefined;
  delete base.prop;
  results.push("no throw");
} catch (e) {
  results.push(nameOf(e));
}

try {
  let base: any = null;
  delete base[0];
  results.push("no throw");
} catch (e) {
  results.push(nameOf(e));
}

try {
  let base: any = undefined;
  delete base[0];
  results.push("no throw");
} catch (e) {
  results.push(nameOf(e));
}

results.join(",");

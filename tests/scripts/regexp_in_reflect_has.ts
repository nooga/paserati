// expect: true
// `in` (OpIn, pkg/vm/vm.go) unconditionally threw a TypeError for a RegExp
// right-hand side - TypeRegExp was simply missing from OpIn's allowed-type
// list, even though a RegExp is an ordinary object in ECMAScript
// ("test" in /x/ is true in Node). The static checker rejected the same
// expression before it ever reached the VM (isObjectType, pkg/checker/
// expressions.go, had no case for the *types.Primitive RegExp marker type -
// see pkg/types/primitive.go) - a plain (non-`any`) `/x/` literal hit this,
// so a typed case is included below, not just `any`-typed ones. Reflect.has
// (regexp, key) had the matching gap: no TypeRegExp case in reflectHas's
// switch (pkg/builtins/reflect_has.go), acknowledged in its own
// default-case comment, so it answered false unconditionally regardless of
// the key.
const checks: boolean[] = [];

// --- a plain, statically-typed RegExp (not `any`) - exercises the checker
// fix, not just the VM/Reflect.has one ---
const typed = /x/;
checks.push("test" in typed && Reflect.has(typed, "test"));
if ("test" in typed) {
  checks.push(true);
} else {
  checks.push(false);
}

const r: any = /x/g;

// --- own "lastIndex" (lives on the RegExpObject itself, not the side
// table - see reflectDeleteProperty's TypeRegExp case) ---
checks.push("lastIndex" in r && Reflect.has(r, "lastIndex"));

// --- inherited methods from RegExp.prototype ---
checks.push("test" in r && Reflect.has(r, "test"));
checks.push("exec" in r && Reflect.has(r, "exec"));

// --- inherited accessors on RegExp.prototype (source/flags are getters,
// not own data properties on the instance) ---
checks.push("source" in r && Reflect.has(r, "source"));
checks.push("flags" in r && Reflect.has(r, "flags"));
checks.push("global" in r && Reflect.has(r, "global"));

// --- a genuinely absent key ---
checks.push(!("nope" in r) && !Reflect.has(r, "nope"));

// --- a plain own property assigned directly onto the instance (the
// side table OwnPropertiesTable/EnsureOwnPropertiesTable manages) ---
r.custom = 42;
checks.push("custom" in r && Reflect.has(r, "custom"));

// --- a RegExp with no `g` flag behaves the same way ---
const r2: any = /y/;
checks.push("lastIndex" in r2 && Reflect.has(r2, "lastIndex"));
checks.push("test" in r2 && Reflect.has(r2, "test"));
checks.push(!("nope" in r2) && !Reflect.has(r2, "nope"));

// --- well-known symbols RegExp.prototype defines (@@match/@@matchAll/
// @@replace/@@search/@@split - pkg/builtins/regexp_init.go): the
// symbol-key switch in OpIn/reflectHas used to fall to `false`
// unconditionally for a RegExp target, even though Node finds these. A
// symbol RegExp.prototype does NOT define (@@iterator) still correctly
// answers false. ---
checks.push(Symbol.match in r && Reflect.has(r, Symbol.match));
checks.push(Symbol.matchAll in r && Reflect.has(r, Symbol.matchAll));
checks.push(Symbol.replace in r && Reflect.has(r, Symbol.replace));
checks.push(Symbol.search in r && Reflect.has(r, Symbol.search));
checks.push(Symbol.split in r && Reflect.has(r, Symbol.split));
checks.push(!(Symbol.iterator in r) && !Reflect.has(r, Symbol.iterator));

// --- a RegExp subclass instance: RegExpObject.prototype ("per-instance
// [[Prototype]] override for subclassing") must be walked instead of
// unconditionally using the intrinsic RegExp.prototype, or an own method
// added on the subclass's prototype (not found on RegExp.prototype
// itself) would incorrectly report absent. ---
class MyRegExp extends RegExp {
  mine() {
    return 1;
  }
}
const sub: any = new MyRegExp("z");
checks.push("mine" in sub && Reflect.has(sub, "mine"));
checks.push("test" in sub && Reflect.has(sub, "test")); // still inherited beyond the subclass
checks.push(!("nope" in sub) && !Reflect.has(sub, "nope"));

// --- a Proxy wrapping a RegExp subclass instance, with no `has` trap:
// `in` (via proxyHasPropertyFallback) and Reflect.has (which recurses
// back into reflectHas for the target) must agree with each other, and
// both must still see the subclass's own prototype method - not just
// the intrinsic RegExp.prototype either fallback used to hardcode. ---
const proxied: any = new Proxy(sub, {});
checks.push("mine" in proxied && Reflect.has(proxied, "mine"));
checks.push("test" in proxied && Reflect.has(proxied, "test"));
checks.push(!("nope" in proxied) && !Reflect.has(proxied, "nope"));

checks.every((c) => c === true);

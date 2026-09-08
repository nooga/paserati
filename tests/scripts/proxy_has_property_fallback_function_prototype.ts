// expect: true
// vm.proxyHasPropertyFallback's TypeFunction/TypeClosure cases (pkg/vm/vm.go)
// used to guard the FunctionPrototype lookup with
// `if vm.FunctionPrototype.Type() == TypeObject { ... }; return false` -
// silently false whenever FunctionPrototype is (as it always is at
// runtime, per pkg/builtins/function_init.go) a TypeNativeFunctionWithProps
// rather than a TypeObject. This fallback is what `in`/Reflect.has use for
// a Proxy wrapping a Function/Closure with no `has` trap, so a
// STRING-keyed property that only exists on Function.prototype (e.g.
// "call") was invisible through such a proxy even though it's found
// correctly on the unwrapped function itself.
//
// Fixed by reusing hasFunctionPrototypeProperty here too, instead of a
// duplicated, incomplete inline check.
//
// This covers string keys only: OpIn's SYMBOL-key switch has no
// `case TypeProxy` at all (a separate, much larger pre-existing gap -
// `anySymbol in proxyObject` is unconditionally false regardless of a
// `has` trap or the target's own properties - flagged independently, not
// fixed here, tracked as task_42fe36b9).

const checks: boolean[] = [];

function fn() {}
const proxiedFn: any = new Proxy(fn, {});

// --- A string-keyed FunctionPrototype method, through the fallback
// (no `has` trap on this proxy). `in` was the broken half before this fix
// (unconditionally false here); Reflect.has(proxiedFn, "call") went
// through the separate proxyReflectHas path and was already correct, so
// it doesn't by itself exercise this fix - the first assertion below
// does. ---
checks.push("call" in proxiedFn);
checks.push(Reflect.has(proxiedFn, "call"));
checks.push("apply" in proxiedFn);

// --- A genuinely absent property is still false through the fallback. ---
checks.push(!("nonexistentMethod" in proxiedFn));

// --- The same, wrapping a Closure (a function with an upvalue) instead of
// a plain Function. ---
function makeClosure() {
  let captured = 1;
  return function closureFn() {
    return captured;
  };
}
const proxiedClosure: any = new Proxy(makeClosure(), {});
checks.push("call" in proxiedClosure);
checks.push(!("nonexistentMethod2" in proxiedClosure));

checks.every((c) => c === true);

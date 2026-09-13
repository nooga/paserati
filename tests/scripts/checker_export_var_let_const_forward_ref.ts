// expect: true
// Type-checker companion to module_export_var_forward_ref.ts.
//
// The checker's Pass 2 pre-pass hoists top-level Let/Const/Var names so a
// function defined earlier in the file can forward-reference one declared
// later - but it only recognized bare statements, never one wrapped in
// ExportNamedDeclaration. So `export var`/`export let`/`export const` never
// got hoisted, and each forward reference below used to fail with
// "Cannot find name", even without any cross-module import involved.
function useVar() {
  return Kind.Number;
}
export var Kind;
(function (K) {
  K["Number"] = "number";
})(Kind || (Kind = {}));

function useLet(): number {
  return letValue;
}
export let letValue = 10;

function useConst(): number {
  return constValue;
}
export const constValue = 32;

useVar() === "number" && useLet() + useConst() === 42;

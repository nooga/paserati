// Helper for module_export_var_forward_ref.ts - see paserati#451.
//
// `makeThing` is defined textually before `Kind`, but `var` (and the
// TS-enum-compiles-to-this exact shape) is hoisted: a real JS engine
// resolves `Kind` fine here, whether this file is the entry script or
// (as in the paired test) loaded as an imported module.
function makeThing() {
  return Kind.Number;
}

export var Kind;
(function (K) {
  K["Number"] = "number";
})(Kind || (Kind = {}));

export { makeThing };

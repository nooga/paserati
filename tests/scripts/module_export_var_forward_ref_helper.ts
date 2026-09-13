// Helper for module_export_var_forward_ref.ts (paserati#451).
// A top-level function forward-references `Kind`, an `export var` declared
// later in this same file - ordinary var hoisting, valid in real JS/TS.
export function makeThing(): string {
  return Kind.Number;
}

export var Kind: any;
(function (K: any) {
  K["Number"] = "number";
})(Kind || (Kind = {}));

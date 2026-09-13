// paserati#452 - the original real-world repro: a rest-parameter-splicing
// wrapper (vnopts' normalizeHandler, from real prettier) spreads
// `args.slice(...)` results into a call. When a slice happened to produce
// exactly one numeric element, the NewArrayWithArgs bug (see
// array_single_number_element_not_sparse.ts) turned that single-argument
// spread into a spread of N holes, shifting every subsequent argument over
// and corrupting the whole call.
//
// Needs a factory called twice, with the SECOND closure's body being a bare
// `value === void 0 || (...)` short-circuit expression, invoked through the
// spread-splicing wrapper - removing any one precondition made the original
// bug not reproduce (see the issue for why), even though the true root
// cause turned out to be nowhere near closures or registers at all.
// expect: B:5
// no-typecheck

function normalizeHandler(handler, superSchema, handlerArgumentsLength) {
  return (...args) =>
    handler(...args.slice(0, handlerArgumentsLength - 1), superSchema, ...args.slice(handlerArgumentsLength - 1));
}

function optionInfoToSchema(exceptionFn) {
  let validateFn;
  if (exceptionFn) {
    validateFn = (value, schema, utils) => "A:" + value;
  } else {
    validateFn = (value, schema, utils) => (value === void 0 || ("B:" + value));
  }
  return normalizeHandler(validateFn, { tag: "s" }, 2);
}

const f1 = optionInfoToSchema((v) => false);
f1("babel", { u: 1 }); // sanity: "A:babel"

const f2 = optionInfoToSchema(null);
f2(5, { u: 2 });

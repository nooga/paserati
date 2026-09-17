// skip-typecheck
// Analogous to nested_object_literal_register_pressure.ts, but for a chain
// of single-element array literals: compileArrayLiteralSimple's own `hint`
// (the array's own reference) is held across every element's recursive
// compile the same way compileObjectLiteral's is - a chain N levels deep
// held N registers just to move a reference back into a register moments
// later. Depth alone used to fail around ~254 levels with "no registers
// available for chunking" (a trivial 1-element array still routes into the
// chunking path once available/2 < 1). See #471.
// (skip-typecheck: as above, the checker's own inference over a 300-
// level nested array type is a separate, pre-existing slow path.)
// expect: 42

function repro() {
  let x = [[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[42]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]];
  for (let i = 0; i < 300; i++) {
    x = x[0];
  }
  return x;
}

repro();

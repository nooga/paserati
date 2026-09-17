// skip-typecheck
// A property whose value is itself an object/array literal shares the
// enclosing object literal's own register file (unlike a method or function
// body, which gets its own) - so compileObjectLiteral's own reference
// (hint, plus keyReg for a computed key) stayed live for the ENTIRE
// recursive compile of a nested value, purely to be reused afterward for
// that property's own store. A chain N levels deep held up to two registers
// per level - real registers, not spillable - just to move a reference back
// moments later, exhausting the 255-register-per-function budget at
// surprisingly shallow depths (computed keys cost twice as much per level
// as a static key, since keyReg needs the same treatment). This mirrors
// #470's chained-assignment fix, applied to literal nesting instead. See #471.
// (skip-typecheck: a 300-level structurally-nested object type is also
// pathologically slow for the checker's own inference to walk - a separate,
// pre-existing concern this test isn't about; it's about the compiler/VM.)
// expect: 42

function repro() {
  const k = "a";
  let x = {[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:{[k]:42}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}};
  for (let i = 0; i < 300; i++) {
    x = x[k];
  }
  return x;
}

repro();

// expect_compile_error: Type 'number' is not assignable to type 'string'
// Hoisted function declarations are checked in source order, so each body
// sees the inferred return type of the one before it. They used to be
// checked in map order, which made the diagnostics differ between runs.
function h() { return 1; }
function g1() { return h(); }
function g2() { return g1(); }
function g3() { return g2(); }
function g4() { return g3(); }
function g5() { return g4(); }
function g6() { return g5(); }
function g7() { return g6(); }
function g8() { return g7(); }
function g9() { return g8(); }
function g10() { return g9(); }
const x: string = g10();

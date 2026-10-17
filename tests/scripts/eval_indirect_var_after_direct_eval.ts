// expect: number 2 number 3
// no-typecheck
// paserati#590: a direct eval called from strict code inherits strictness,
// which must not stick to the compiler. Left set, every later indirect eval
// compiled as strict code and its vars stayed eval-local instead of becoming
// globals.

eval("1");
(0, eval)("var after590a = 2");
function strictCaller590() { "use strict"; eval("1"); }
strictCaller590();
(0, eval)("var after590b = 3");
typeof after590a + " " + globalThis.after590a + " " + typeof after590b + " " + globalThis.after590b;

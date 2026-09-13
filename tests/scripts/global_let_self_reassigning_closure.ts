// expect: 1
// no-typecheck
// paserati#449: a top-level `let`-bound closure that reassigns its own
// binding from inside its own body (the standard "lazily replace yourself
// the first time you're called" memoization idiom - e.g. the real,
// unmodified `internal/utils/uuid.mjs` shipped by both @anthropic-ai/sdk and
// openai) used to fail to compile at all ("Invalid local register index 255
// for upvalue capture"), and a first fix attempt "fixed" it into silently
// writing the wrong place - a(): the closure's own self-reference resolved to
// a fresh local register instead of the real global binding, so calling a()
// looked like it worked but `a` afterward was still the *original* closure,
// not 1. The reassignment must actually land in the same global slot every
// other reference to `a` reads from.
let a = function () {
  a = 1;
};
a();
a;

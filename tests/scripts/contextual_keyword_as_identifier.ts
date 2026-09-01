// expect: 4
// paserati#164: `as` is a contextual keyword (type assertions), not a
// fully reserved word - it must also parse as a plain identifier
// reference, the way `satisfies`/`of`/`from`/`type` already do.
const as = 1;
function f(as: number): number {
  return as;
}
let asserted = 2 as number;
// The actual prefix-then-infix collision this fix creates: `as` parsed as
// a plain identifier reference, immediately followed by the `as` type
// assertion operator reusing the very same token.
let collision = as as number;
f(as) + asserted + collision;

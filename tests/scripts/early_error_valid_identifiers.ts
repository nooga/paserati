// expect: 6
// Identifier early errors must not reject these valid uses: escaped reserved
// words as property names, yield/await as plain identifiers outside
// generators/async code, and a class expression named await is fine in an
// arrow body inside a static block.
const o = { \u0062reak: 1, yield: 2 };
function plain(yield, await) { return yield + await; }
let n = 0;
class C { static { (() => { class await {} n = 1; })(); } }
o.\u0062reak + o.yield + plain(1, 1) + n;

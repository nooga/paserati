// An interface naming itself in its own heritage stays the same type:
// BB.x.x is BB, so BB relates to AA<AA<BB>> and AA<BB> to AA<AA<BB>>.
// expect: 3
interface AA<T extends AA<T>> { x: T }
interface BB extends AA<AA<BB>> {}
interface P<T> { x: T }
interface Q extends P<P<Q>> { n: number }
const ab: AA<BB> = { x: null as any };
const wide: AA<AA<BB>> = ab;
const leaf: Q = { n: 3, x: { x: null as any } };
function back(l: Q): Q { return l.x.x; }
function asBase(l: Q): P<P<Q>> { return back(l); }
leaf.n;

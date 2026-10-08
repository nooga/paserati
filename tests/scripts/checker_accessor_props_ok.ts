// expect: 4|2|9|ok|a
// Accessors as properties: static getters are values, getter+setter pairs
// are writable, and both halves of an object-literal pair are checked.
class Sq {
  private static _n = 1;
  static get count(): number { return 2; }
  static get n(): number { return Sq._n; }
  static set n(v: number) { Sq._n = v; }
  constructor(private s: number) {}
  get side(): number { return this.s; }
  set side(v: number) { this.s = v; }
}
const q = new Sq(2);
q.side = 3;
Sq.n = 9;
const c: number = Sq.count;
let log = "";
const o = { get v() { return "ok"; }, set v(x: string) { log = x; } };
o.v = "a";
const D = ["a", "b"] as const;
type Dir = typeof D[number];
const d: Dir = "a";
`${q.side + 1}|${c}|${Sq.n}|${o.v}|${log || d}`;

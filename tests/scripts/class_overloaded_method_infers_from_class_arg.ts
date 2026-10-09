// expect: 5
class Base<D, S> { constructor(public definition: D, public state: S) {} }
class X extends Base<{ a: string }, { b?: number }> {}
class T0 {
    over<T extends { definition: any; state: any }>(cls: new (...args: any[]) => T, def: T["definition"]): T["state"];
    over<D, S>(name: string, def: D): S;
    over(x: any, def: any): any { return { b: 5 }; }
}
const t = new T0();
const s2: { b?: number } = t.over(X, { a: "x" });
s2.b;

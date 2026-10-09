// expect: true
class C {
    f(x: number): number;
    f<A, B>(x: string, a: A): B;
    f(x: any, a?: any): any { return a === "a"; }
}
const r: boolean = new C().f<string, boolean>("s", "a");
r;

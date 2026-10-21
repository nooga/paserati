// expect: 6
// An interface may extend a namespace member declared further down.
interface A extends N.B {}
function useA(a: A): number {
  return a.x;
}
namespace N {
  export interface B {
    x: number;
  }
}
// Or an interface declared further down, generic or not.
interface D1 extends Later {}
interface D2<T> extends LaterG<T, string> {
  extra?: number;
}
function useD(d1: D1, d2: D2<number>): number {
  const s: string = d2.r;
  return d1.x + d2.t + s.length;
}
interface Later {
  x: number;
}
interface LaterG<T, R> {
  t: T;
  r: R;
}
// `this` in an inherited generic method is the inheriting type.
interface P {
  on<K>(n: K): this;
}
interface C extends P {
  own(): number;
}
function useC(c: C): number {
  return c.on(1).own();
}
// `extends Base` with no type arguments takes Base's defaults.
class Base<T = number, U = T[]> {
  items: U = [] as any;
  first(): T | undefined {
    return (this.items as any)[0];
  }
}
class Derived extends Base {}
const d = new Derived();
const xs: number[] = d.items;
xs.push(6);
d.first()!;

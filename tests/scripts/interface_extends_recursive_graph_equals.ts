// A derived interface's 'this'-rebound copy of a densely mutually recursive
// graph (playwright's ChromiumBrowserContext extends BrowserContext) is
// compared with the original; without memoized pairs ObjectType.Equals
// re-proved every pair once per path to it and never finished.
// expect: true
interface T0 { m0(): T1; m1(): T4; m2(): T7; on(e: 'x', f: (p: T1) => void): this; }
interface T1 { m0(): T8; m1(): T11; m2(): T2; on(e: 'x', f: (p: T2) => void): this; }
interface T2 { m0(): T3; m1(): T6; m2(): T9; on(e: 'x', f: (p: T3) => void): this; }
interface T3 { m0(): T10; m1(): T1; m2(): T4; on(e: 'x', f: (p: T4) => void): this; }
interface T4 { m0(): T5; m1(): T8; m2(): T11; on(e: 'x', f: (p: T5) => void): this; }
interface T5 { m0(): T0; m1(): T3; m2(): T6; on(e: 'x', f: (p: T6) => void): this; }
interface T6 { m0(): T7; m1(): T10; m2(): T1; on(e: 'x', f: (p: T7) => void): this; }
interface T7 { m0(): T2; m1(): T5; m2(): T8; on(e: 'x', f: (p: T8) => void): this; }
interface T8 { m0(): T9; m1(): T0; m2(): T3; on(e: 'x', f: (p: T9) => void): this; }
interface T9 { m0(): T4; m1(): T7; m2(): T10; on(e: 'x', f: (p: T10) => void): this; }
interface T10 { m0(): T11; m1(): T2; m2(): T5; on(e: 'x', f: (p: T11) => void): this; }
interface T11 { m0(): T6; m1(): T9; m2(): T0; on(e: 'x', f: (p: T0) => void): this; }
interface Sub extends T0 {}
const c: Sub | null = null;
c === null;

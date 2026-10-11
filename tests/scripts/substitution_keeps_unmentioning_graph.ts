// Instantiating a generic whose members reach a large interface graph
// (playwright's Page/Frame/Locator with @types/node) must keep the parts the
// substitution can't change. Those parts used to count as mentioning T: an
// instantiated Handle<T>'s `evaluate<R, O extends T = T>` constraint, the
// `infer T` in Unboxed<Arg>, `Promise<X>` references. So every
// instantiation copied, and lazily re-substituted, the whole graph.
// expect: true
type Unboxed<Arg> = Arg extends Handle<infer T> ? T : Arg extends [infer A0] ? [Unboxed<A0>] : Arg;
interface Handle<T> { evaluate<R, O extends T = T>(f: (o: O, a: Unboxed<R>) => R): Promise<R>; json(): Promise<T>; }
interface N0 { m0(): Promise<N1>; m1(): Promise<N4>; m2(): Promise<N7>; handle(): Handle<N1>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N1 { m0(): Promise<N8>; m1(): Promise<N11>; m2(): Promise<N14>; handle(): Handle<N2>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N2 { m0(): Promise<N15>; m1(): Promise<N18>; m2(): Promise<N21>; handle(): Handle<N3>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N3 { m0(): Promise<N22>; m1(): Promise<N25>; m2(): Promise<N28>; handle(): Handle<N4>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N4 { m0(): Promise<N29>; m1(): Promise<N2>; m2(): Promise<N5>; handle(): Handle<N5>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N5 { m0(): Promise<N6>; m1(): Promise<N9>; m2(): Promise<N12>; handle(): Handle<N6>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N6 { m0(): Promise<N13>; m1(): Promise<N16>; m2(): Promise<N19>; handle(): Handle<N7>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N7 { m0(): Promise<N20>; m1(): Promise<N23>; m2(): Promise<N26>; handle(): Handle<N8>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N8 { m0(): Promise<N27>; m1(): Promise<N0>; m2(): Promise<N3>; handle(): Handle<N9>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N9 { m0(): Promise<N4>; m1(): Promise<N7>; m2(): Promise<N10>; handle(): Handle<N10>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N10 { m0(): Promise<N11>; m1(): Promise<N14>; m2(): Promise<N17>; handle(): Handle<N11>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N11 { m0(): Promise<N18>; m1(): Promise<N21>; m2(): Promise<N24>; handle(): Handle<N12>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N12 { m0(): Promise<N25>; m1(): Promise<N28>; m2(): Promise<N1>; handle(): Handle<N13>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N13 { m0(): Promise<N2>; m1(): Promise<N5>; m2(): Promise<N8>; handle(): Handle<N14>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N14 { m0(): Promise<N9>; m1(): Promise<N12>; m2(): Promise<N15>; handle(): Handle<N15>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N15 { m0(): Promise<N16>; m1(): Promise<N19>; m2(): Promise<N22>; handle(): Handle<N16>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N16 { m0(): Promise<N23>; m1(): Promise<N26>; m2(): Promise<N29>; handle(): Handle<N17>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N17 { m0(): Promise<N0>; m1(): Promise<N3>; m2(): Promise<N6>; handle(): Handle<N18>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N18 { m0(): Promise<N7>; m1(): Promise<N10>; m2(): Promise<N13>; handle(): Handle<N19>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N19 { m0(): Promise<N14>; m1(): Promise<N17>; m2(): Promise<N20>; handle(): Handle<N20>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N20 { m0(): Promise<N21>; m1(): Promise<N24>; m2(): Promise<N27>; handle(): Handle<N21>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N21 { m0(): Promise<N28>; m1(): Promise<N1>; m2(): Promise<N4>; handle(): Handle<N22>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N22 { m0(): Promise<N5>; m1(): Promise<N8>; m2(): Promise<N11>; handle(): Handle<N23>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N23 { m0(): Promise<N12>; m1(): Promise<N15>; m2(): Promise<N18>; handle(): Handle<N24>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N24 { m0(): Promise<N19>; m1(): Promise<N22>; m2(): Promise<N25>; handle(): Handle<N25>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N25 { m0(): Promise<N26>; m1(): Promise<N29>; m2(): Promise<N2>; handle(): Handle<N26>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N26 { m0(): Promise<N3>; m1(): Promise<N6>; m2(): Promise<N9>; handle(): Handle<N27>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N27 { m0(): Promise<N10>; m1(): Promise<N13>; m2(): Promise<N16>; handle(): Handle<N28>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N28 { m0(): Promise<N17>; m1(): Promise<N20>; m2(): Promise<N23>; handle(): Handle<N29>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface N29 { m0(): Promise<N24>; m1(): Promise<N27>; m2(): Promise<N0>; handle(): Handle<N0>; $eval<R>(sel: string, f: (e: Unboxed<R>) => R): Promise<R>; }
interface Wrap<T> { value: T; root: N0; }
const r1: Wrap<N0>["root"] | null = null as Wrap<N1>["root"] | null;
const r2: Wrap<N1>["root"] | null = null as Wrap<N2>["root"] | null;
const r3: Wrap<N2>["root"] | null = null as Wrap<N3>["root"] | null;
const r4: Wrap<N3>["root"] | null = null as Wrap<N4>["root"] | null;
const r5: Wrap<N4>["root"] | null = null as Wrap<N5>["root"] | null;
const r6: Wrap<N5>["root"] | null = null as Wrap<N6>["root"] | null;
const r7: Wrap<N6>["root"] | null = null as Wrap<N7>["root"] | null;
const r8: Wrap<N7>["root"] | null = null as Wrap<N8>["root"] | null;
const r9: Wrap<N8>["root"] | null = null as Wrap<N9>["root"] | null;
const r10: Wrap<N9>["root"] | null = null as Wrap<N10>["root"] | null;
const r11: Wrap<N10>["root"] | null = null as Wrap<N11>["root"] | null;
const r12: Wrap<N11>["root"] | null = null as Wrap<N12>["root"] | null;
const r13: Wrap<N12>["root"] | null = null as Wrap<N13>["root"] | null;
const r14: Wrap<N13>["root"] | null = null as Wrap<N14>["root"] | null;
const r15: Wrap<N14>["root"] | null = null as Wrap<N15>["root"] | null;
const r16: Wrap<N15>["root"] | null = null as Wrap<N16>["root"] | null;
const r17: Wrap<N16>["root"] | null = null as Wrap<N17>["root"] | null;
const r18: Wrap<N17>["root"] | null = null as Wrap<N18>["root"] | null;
const r19: Wrap<N18>["root"] | null = null as Wrap<N19>["root"] | null;
const r20: Wrap<N19>["root"] | null = null as Wrap<N20>["root"] | null;
const r21: Wrap<N20>["root"] | null = null as Wrap<N21>["root"] | null;
const r22: Wrap<N21>["root"] | null = null as Wrap<N22>["root"] | null;
const r23: Wrap<N22>["root"] | null = null as Wrap<N23>["root"] | null;
const r24: Wrap<N23>["root"] | null = null as Wrap<N24>["root"] | null;
const r25: Wrap<N24>["root"] | null = null as Wrap<N25>["root"] | null;
const r26: Wrap<N25>["root"] | null = null as Wrap<N26>["root"] | null;
const r27: Wrap<N26>["root"] | null = null as Wrap<N27>["root"] | null;
const r28: Wrap<N27>["root"] | null = null as Wrap<N28>["root"] | null;
const r29: Wrap<N28>["root"] | null = null as Wrap<N29>["root"] | null;
r1 === null;

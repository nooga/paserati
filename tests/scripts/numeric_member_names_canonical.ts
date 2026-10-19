// expect: 11
interface T2 { 1.0: string }
var s2: { 1: string } = null as any as T2;
type T3 = { 0x10: number };
const t3: { 16: number } = { 16: 1 } as T3;
class C { 1.0 = 5; 0x10 = 6 }
const c: any = new C();
c["1"] + c["16"];

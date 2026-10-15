// expect: 3
// A mapped type in a member (tailwind's `theme: Partial<ThemeConfig &
// { extend: Partial<ThemeConfig> }>`) is related by its expansion.
interface Base { a: string; b: number }
const x: { e: Partial<Base> } = { e: {} };
const y: { e?: Readonly<Base> } = { e: { a: "y", b: 2 } };
const t: Partial<Base & { extend: Partial<Base> }> = { extend: { b: 1 } };
(t.extend!.b ?? 0) + y.e!.b + Object.keys(x.e).length;

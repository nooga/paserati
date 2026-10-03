// expect: hello|4|x|0,1,2,3
// no-typecheck
const bytes = new TextEncoder().encode("xxhelloyy");
const view = new DataView(bytes.buffer, 2, 5);
const s = new TextDecoder().decode(view);
const a: any[] = [0, 1, 2];
a[3n as any] = "x";
s + "|" + a.length + "|" + a[3] + "|" + Object.keys(a).join(",");

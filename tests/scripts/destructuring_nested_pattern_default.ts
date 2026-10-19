// expect: 1|{"y":2}|[2]|5{"y":3}|7[2,3]
let a: any, b: any, c: any = { x: { a: 1, y: 2 } }, d: any;
({ x: { a, ...b } = d } = c);
const r1 = a + "|" + JSON.stringify(b);
let e: any;
({ x: [a, ...e] = d } = { x: [1, 2] });
const r2 = JSON.stringify(e);
let f: any, g: any;
[{ a: f = 5, ...g } = { y: 3 }] = [];
const r3 = f + JSON.stringify(g);
let h: any, i: any;
({ q: [h = 7, , ...i] = [undefined, 1, 2, 3] } = {});
const r4 = h + JSON.stringify(i);
r1 + "|" + r2 + "|" + r3 + "|" + r4;

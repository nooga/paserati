// expect: ok

let obj: { readonly x: number; y?: string } = { x: 1 };
let pair: [number, string] = [1, "a"];

pair.push(2);

obj.x === 1 && pair.length === 3 ? "ok" : "fail";

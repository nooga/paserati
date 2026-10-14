// expect: 3
// Spreading an any value gives any (#705), so its properties can be read.
const parsed: any = JSON.parse('{"x":2}');
const copy = { ...parsed, y: 1 };
const n: number = copy.x + copy.y;
n;

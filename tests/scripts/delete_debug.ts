// expect: true

let obj: { x?: number } = { x: 10 };
delete obj.x;
obj.x === undefined;
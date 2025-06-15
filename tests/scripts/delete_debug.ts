// expect: true

let obj = { x: 10 };
delete obj.x;
obj.x === undefined;
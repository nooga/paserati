// Test object rest elements contains remaining properties
// (the rest target keeps its declared type, as in tsc)
let x = 0;
let rest: { y?: number; z?: number } = {};
({x, ...rest} = {x: 10, y: 20, z: 30});
rest.y;
// expect: 20

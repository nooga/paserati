// Test basic object rest elements in assignment
let x = 0;
let rest = {};
({x, ...rest} = {x: 10, y: 20, z: 30});
x;
// expect: 10
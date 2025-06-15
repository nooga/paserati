// Test object rest elements contains remaining properties  
let x = 0;
let rest = {};
{x, ...rest} = {x: 10, y: 20, z: 30};
rest.y;
// expect: 20
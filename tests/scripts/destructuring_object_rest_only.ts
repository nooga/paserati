// Test rest-only object destructuring
let {...everything} = {x: 10, y: 20};
everything.x;
// expect: 10
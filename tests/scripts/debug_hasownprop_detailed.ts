// expect: true

let obj: any = {x: 42};
// First check if the property exists on the object
let hasX = 'x' in obj;
// Then check hasOwnProperty
let ownX = obj.hasOwnProperty('x');
// Both should be true
hasX && ownX;
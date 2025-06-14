// expect: true

// Test hasOwnProperty on regular object literal
let regularObj: any = {x: 42};
let test1 = regularObj.hasOwnProperty('x');

// Test hasOwnProperty on Object.create object  
let proto: any = {y: 100};
let createdObj: any = Object.create(proto);
createdObj.z = 200;
let test2 = createdObj.hasOwnProperty('z');
let test3 = !createdObj.hasOwnProperty('y'); // should NOT have own property y

// All should be true
test1 && test2 && test3;
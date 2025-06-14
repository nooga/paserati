// expect: true

// Create an array and call concat
let arr: any = [1, 2, 3];
console.log("arr.concat([4,5]) =", arr.concat([4, 5]));

// Create an object and call hasOwnProperty
let obj: any = {};
obj.foo = 42;
console.log('obj.hasOwnProperty("foo") =', obj.hasOwnProperty("foo"));

// Final statement to satisfy test runner
obj.hasOwnProperty("foo");

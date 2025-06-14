// expect: true

// // Test basic bind - binding 'this' value
// function greet(this: {name: string}, greeting: string) {
//     return greeting + " " + this.name;
// }

// let person = {name: "John"};
// let boundGreet = greet.bind(person);
// let test1 = boundGreet("Hello") === "Hello John";

// // Test bind with partial application
// function add(a: number, b: number, c: number) {
//     return a + b + c;
// }

// let add5 = add.bind(null, 5);
// let test2 = add5(3, 2) === 10;

// let add5and3 = add.bind(null, 5, 3);
// let test3 = add5and3(2) === 10;

// // Test bind with method
// let obj = {
//     x: 42,
//     getX: function() {
//         return this.x;
//     }
// };

// let unboundGetX = obj.getX;
// let boundGetX = unboundGetX.bind(obj);
// let test4 = boundGetX() === 42;

// // Test bind with different 'this' value
// let obj2 = {x: 100};
// let boundToObj2 = obj.getX.bind(obj2);
// let test5 = boundToObj2() === 100;

// // All tests should pass
// test1 && test2 && test3 && test4 && test5;

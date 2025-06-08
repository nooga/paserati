// expect: true
// Basic prototype chain test
function Person(name: string) {
  this.name = name;
}

// Add method to prototype
Person.prototype.greet = function () {
  return "Hello, " + this.name;
};

// Create instance
let john = new Person("John");

// Test own property
console.log(john.name); // John

// Test prototype method
console.log(john.greet()); // Hello, John

// Test instanceof
console.log(john instanceof Person); // true
console.log(john instanceof Object); // true

// Test hasOwnProperty
console.log("name" in john); // true
console.log("greet" in john); // true (final expression)

true;

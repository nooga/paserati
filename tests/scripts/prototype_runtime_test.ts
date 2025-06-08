// expect: true
// Runtime test without type annotations
let Person = function (name) {
  this.name = name;
};

Person.prototype.greet = function () {
  return "Hello, " + this.name;
};

let john = new Person("John");
console.log(john.name);
console.log(john.greet());
console.log(john instanceof Person);

true;

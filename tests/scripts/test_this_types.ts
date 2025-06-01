// expect: John Doe
// Test 'this' keyword type checking in various contexts

let person = {
  firstName: "John",
  lastName: "Doe",
  age: 30,
  getFullName: function () {
    return this.firstName + " " + this.lastName;
  },
  getAge: function () {
    return this.age;
  },
  incrementAge: function () {
    this.age = this.age + 1;
    return this.age;
  },
};

person.getFullName();

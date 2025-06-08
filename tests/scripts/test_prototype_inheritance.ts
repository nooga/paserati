// expect: function
// Test that prototype inheritance still works with explicit this parameter

function Person(this: { name: string; age: number }, name: string, age: number) {
    this.name = name;
    this.age = age;
}

Person.prototype.greet = function() {
    return this.name + " is " + this.age + " years old";
};

let alice = new Person("Alice", 30);
typeof alice.greet
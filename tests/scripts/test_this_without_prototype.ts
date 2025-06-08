// Test explicit this parameter without prototype manipulation

// expect: {name: "Alice", age: 30}

interface Person { name: string; age: number }

function createPerson(this: Person, name: string, age: number) {
    this.name = name;
    this.age = age;
}

let person = new createPerson("Alice", 30);
person
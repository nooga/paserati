// expect: John

// Test type assertion with interfaces
interface Person {
    name: string;
    age: number;
}

let obj: unknown = { name: "John", age: 30 };
let person = obj as Person;
person.name;
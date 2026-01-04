// Test Paserati.reflect<T>() with object properties inspection
// expect: Person has 2 properties: name is string, age is number

interface Person {
    name: string;
    age: number;
}

const personType = Paserati.reflect<Person>();

// Access properties by name (not relying on iteration order)
const props = personType.properties;
const nameType = props.name.type.name;
const ageType = props.age.type.name;

"Person has 2 properties: name is " + nameType + ", age is " + ageType;

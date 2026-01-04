// Test Paserati.reflect<T>() with various types
// expect: string-number-array-object-union

// Primitive types
const strType = Paserati.reflect<string>();
const numType = Paserati.reflect<number>();

// Array type
const arrType = Paserati.reflect<string[]>();

// Object type
interface Person {
    name: string;
    age: number;
}
const objType = Paserati.reflect<Person>();

// Union type
const unionType = Paserati.reflect<string | number>();

strType.name + "-" + numType.name + "-" + arrType.kind + "-" + objType.kind + "-" + unionType.kind;

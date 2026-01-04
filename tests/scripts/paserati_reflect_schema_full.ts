// Test Paserati.reflect<T>().toJSONSchema() with various types
// expect: all schemas generated correctly

interface User {
    name: string;
    age: number;
    email?: string;
}

// Test primitive types
const stringSchema = Paserati.reflect<string>().toJSONSchema();
const numberSchema = Paserati.reflect<number>().toJSONSchema();
const booleanSchema = Paserati.reflect<boolean>().toJSONSchema();

// Test array type
const arraySchema = Paserati.reflect<string[]>().toJSONSchema();

// Test object type
const userSchema = Paserati.reflect<User>().toJSONSchema();

// Test union type
const unionSchema = Paserati.reflect<string | number>().toJSONSchema();

// Verify results
let allCorrect = true;

// Check primitives
if (stringSchema.type !== "string") allCorrect = false;
if (numberSchema.type !== "number") allCorrect = false;
if (booleanSchema.type !== "boolean") allCorrect = false;

// Check array
if (arraySchema.type !== "array") allCorrect = false;
if (arraySchema.items.type !== "string") allCorrect = false;

// Check object has required properties
if (userSchema.type !== "object") allCorrect = false;
if (!userSchema.properties) allCorrect = false;

// Check union has anyOf
if (!unionSchema.anyOf) allCorrect = false;
if (unionSchema.anyOf.length !== 2) allCorrect = false;

allCorrect ? "all schemas generated correctly" : "some schemas failed";

// Test JSON Schema generation with inheritance
// expect: all inheritance tests passed

// Helper to check if array includes value
function arrayIncludes(arr: any, value: string): boolean {
    if (!arr) return false;
    for (let i = 0; i < arr.length; i++) {
        if (arr[i] === value) return true;
    }
    return false;
}

// === Test class inheritance ===
class Animal {
    name: string;
    age?: number;
}

class Dog extends Animal {
    breed: string;
    bark(): void {}
}

const dogSchema = Paserati.reflect<Dog>().toJSONSchema();

// Dog should have: name (required), age (optional), breed (required)
// bark() method should be filtered out
let classInheritanceOk =
    dogSchema.properties?.name?.type === "string" &&
    dogSchema.properties?.age?.type === "number" &&
    dogSchema.properties?.breed?.type === "string" &&
    !dogSchema.properties?.bark && // method should be filtered
    arrayIncludes(dogSchema.required, "name") &&
    arrayIncludes(dogSchema.required, "breed") &&
    !arrayIncludes(dogSchema.required, "age"); // age is optional

// === Test interface inheritance (single extends) ===
interface Named {
    name: string;
}

interface Person extends Named {
    email: string;
}

const personSchema = Paserati.reflect<Person>().toJSONSchema();
let interfaceInheritanceOk =
    personSchema.properties?.name?.type === "string" &&
    personSchema.properties?.email?.type === "string" &&
    arrayIncludes(personSchema.required, "name") &&
    arrayIncludes(personSchema.required, "email");

// === Test interface inheritance (multiple extends) ===
interface Timestamped {
    createdAt: string;
    updatedAt?: string;
}

interface Entity extends Named, Timestamped {
    id: number;
}

const entitySchema = Paserati.reflect<Entity>().toJSONSchema();
let multipleExtendsOk =
    entitySchema.properties?.name?.type === "string" &&
    entitySchema.properties?.id?.type === "number" &&
    entitySchema.properties?.createdAt?.type === "string" &&
    entitySchema.properties?.updatedAt?.type === "string" &&
    arrayIncludes(entitySchema.required, "name") &&
    arrayIncludes(entitySchema.required, "id") &&
    arrayIncludes(entitySchema.required, "createdAt") &&
    !arrayIncludes(entitySchema.required, "updatedAt"); // optional

// === Test deep inheritance chain ===
interface Base {
    baseField: string;
}

interface Middle extends Base {
    middleField: string;
}

interface Top extends Middle {
    topField: string;
}

const topSchema = Paserati.reflect<Top>().toJSONSchema();
let deepInheritanceOk =
    topSchema.properties?.baseField?.type === "string" &&
    topSchema.properties?.middleField?.type === "string" &&
    topSchema.properties?.topField?.type === "string" &&
    topSchema.required?.length === 3;

// === Verify all tests ===
const allPassed = classInheritanceOk && interfaceInheritanceOk && multipleExtendsOk && deepInheritanceOk;
allPassed ? "all inheritance tests passed" : "some tests failed";

// 🚀 Paserati Generics Simple Showcase
// Demonstrating the working features of Paserati's generics implementation

console.log("=== 🎉 PASERATI GENERICS SIMPLE SHOWCASE 🎉 ===");

// ====================================================================
// 1. BASIC GENERIC FUNCTIONS - What works in Paserati
// ====================================================================

console.log("--- Basic Generic Functions ---");

// Identity function with type inference
function identity<T>(x: T): T {
    return x;
}

// Simple generic container
interface Container<T> {
    value: T;
}

// Create containers with different types
let stringContainer: Container<string> = { value: "hello" };
let numberContainer: Container<number> = { value: 42 };
let boolContainer: Container<boolean> = { value: true };

// Test basic generics with type inference
let str = identity("hello");
let num = identity(42);
let bool = identity(true);

console.log("identity('hello'):", str);
console.log("identity(42):", num);
console.log("identity(true):", bool);

console.log("stringContainer.value:", stringContainer.value);
console.log("numberContainer.value:", numberContainer.value);
console.log("boolContainer.value:", boolContainer.value);
console.log();

// ====================================================================
// 2. GENERIC TYPE ALIASES - Working features
// ====================================================================

console.log("--- Generic Type Aliases ---");

type Optional<T> = T | undefined;
type Pair<T, U> = {
    first: T;
    second: U;
};

let maybeString: Optional<string> = "maybe";
let maybeEmpty: Optional<string> = undefined;

let numberStringPair: Pair<number, string> = {
    first: 1,
    second: "one"
};

console.log("maybeString:", maybeString);
console.log("maybeEmpty:", maybeEmpty);
console.log("numberStringPair.first:", numberStringPair.first);
console.log("numberStringPair.second:", numberStringPair.second);
console.log();

// ====================================================================
// 3. GENERIC CONSTRAINTS - Working validation
// ====================================================================

console.log("--- Generic Constraints ---");

interface Lengthable {
    length: number;
}

interface Box<T extends Lengthable> {
    item: T;
}

// This works - object with length property
let validBox: Box<{length: number; data: string}> = {
    item: { length: 5, data: "hello" }
};

console.log("validBox.item.length:", validBox.item.length);
console.log("validBox.item.data:", validBox.item.data);
console.log();

// ====================================================================
// 4. ARRAY GENERICS - Built-in support
// ====================================================================

console.log("--- Array Generics ---");

let stringArray: Array<string> = ["one", "two", "three"];
let numberArray: Array<number> = [1, 2, 3];

console.log("stringArray:", stringArray);
console.log("numberArray:", numberArray);
console.log("stringArray.length:", stringArray.length);
console.log("numberArray.length:", numberArray.length);
console.log();

// ====================================================================
// 5. NESTED GENERICS - Complex types
// ====================================================================

console.log("--- Nested Generic Types ---");

type NestedContainer<T> = Container<Container<T>>;

let nestedString: NestedContainer<string> = {
    value: {
        value: "deeply nested"
    }
};

let innerValue = nestedString.value.value;
console.log("nestedString.value.value:", innerValue);
console.log();

// ====================================================================
// FINALE
// ====================================================================

console.log("--- 🎆 FINALE: What Works in Paserati Generics! 🎆 ---");
console.log("✅ Generic function declarations with type parameters");
console.log("✅ Generic interfaces and type aliases");  
console.log("✅ Generic type constraints with validation");
console.log("✅ Built-in generic types (Array<T>)");
console.log("✅ Nested generic type instantiation");
console.log("✅ Type safety validation");
console.log("✅ Zero runtime overhead (complete type erasure)");
console.log();
console.log("🚀 Paserati Generics: Core functionality working! 🚀");

// expect: deeply nested
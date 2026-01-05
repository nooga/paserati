// Test non-null assertion operator (x!)
// expect: all tests passed

let passed = 0;
let failed = 0;

// Test 1: Basic non-null assertion on nullable string
let str: string | null = "hello";
if (str!.length === 5) {
    passed++;
} else {
    failed++;
}

// Test 2: Non-null assertion on undefined union
let num: number | undefined = 42;
if (num! * 2 === 84) {
    passed++;
} else {
    failed++;
}

// Test 3: Non-null assertion on object property access
interface User {
    name: string;
    age?: number;
}
let user: User | null = { name: "Alice", age: 30 };
if (user!.name === "Alice") {
    passed++;
} else {
    failed++;
}

// Test 4: Non-null assertion on optional property
if (user!.age! === 30) {
    passed++;
} else {
    failed++;
}

// Test 5: Chained non-null assertion
interface Nested {
    inner?: { value: string };
}
let nested: Nested | undefined = { inner: { value: "test" } };
if (nested!.inner!.value === "test") {
    passed++;
} else {
    failed++;
}

// Test 6: Non-null assertion in expression
let maybeNum: number | null = 10;
let result = (maybeNum! + 5) * 2;
if (result === 30) {
    passed++;
} else {
    failed++;
}

// Test 7: Non-null assertion on array element access
let arr: (string | null)[] = ["a", null, "b"];
if (arr[0]!.toUpperCase() === "A") {
    passed++;
} else {
    failed++;
}

// All tests should pass
passed === 7 && failed === 0 ? "all tests passed" : "some tests failed: " + passed + "/" + 7;

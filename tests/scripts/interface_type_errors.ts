// expect_compile_error: Type Error

interface Point {
  x: number;
  y: number;
}

interface Named {
  name: string;
}

// Test 1: Missing property should fail
let incomplete = { x: 10 }; // Missing y property
let shouldFail1: Point = incomplete;

// Test 2: Wrong property type should fail
let wrongType = { x: "not a number", y: 20 };
let shouldFail2: Point = wrongType;

// Test 3: Completely different interface should fail
let named: Named = { name: "test" };
let shouldFail3: Point = named;

shouldFail1;

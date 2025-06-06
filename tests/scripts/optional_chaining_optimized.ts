// Test optional chaining with our OpIsNullish optimization
// This should use 1 register instead of 5+ for null/undefined checks

let obj1: { name?: string } | null = { name: "Alice" };
let obj2: { name?: string } | null = null;
let obj3: { name?: string } | undefined = undefined;
let obj4: { name?: string } = {};

// Basic optional chaining (should use OpIsNullish)
let result1 = obj1?.name; // "Alice"
let result2 = obj2?.name; // undefined
let result3 = obj3?.name; // undefined
let result4 = obj4?.name; // undefined

// Chaining with arrays
let arr1: number[] | null = [1, 2, 3];
let arr2: number[] | null = null;
let length1 = arr1?.length; // 3
let length2 = arr2?.length; // undefined

// Complex chaining scenarios
let nested: { inner?: { value?: string } } | null = {
  inner: { value: "deep" },
};
let nestedNull: { inner?: { value?: string } } | null = null;
let deepValue1 = nested?.inner?.value; // "deep"
let deepValue2 = nestedNull?.inner?.value; // undefined

// expect: Alice
result1;

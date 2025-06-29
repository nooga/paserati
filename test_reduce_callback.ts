// Test for array reduce callback signature issue

// Test 1: Basic reduce with 2-parameter callback
const numbers = [1, 2, 3, 4, 5];
const sum = numbers.reduce((acc, val) => acc + val, 0);
console.log("Sum:", sum);

// Test 2: Reduce with object accumulator
interface User {
  name: string;
  age: number;
}

const users: User[] = [
  { name: "Alice", age: 25 },
  { name: "Bob", age: 30 },
  { name: "Charlie", age: 35 }
];

// This is the pattern from the power showcase that's failing
const summary = users.reduce(
  (acc, user) => {
    return {
      ...acc,
      totalAge: acc.totalAge + user.age,
      count: acc.count + 1
    };
  },
  { totalAge: 0, count: 0 }
);

console.log("Summary:", summary);

// Test 3: Full signature (should also work)
const sumWithIndex = numbers.reduce((acc, val, index, array) => {
  console.log(`Processing index ${index} of ${array.length}`);
  return acc + val;
}, 0);

console.log("Sum with index:", sumWithIndex);

// expect: Sum: 15
// expect: Summary: { totalAge: 90, count: 3 }
// expect: Processing index 0 of 5
// expect: Processing index 1 of 5
// expect: Processing index 2 of 5
// expect: Processing index 3 of 5
// expect: Processing index 4 of 5
// expect: Sum with index: 15
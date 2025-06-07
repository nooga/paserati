// expect: ok

// Basic for...of with number array
let numbers = [1, 2, 3, 4, 5];
console.log("Numbers:");
for (let num of numbers) {
  console.log(num);
}

// for...of with string array
let words = ["hello", "world", "typescript"];
console.log("\nWords:");
for (let word of words) {
  console.log(word);
}

// for...of with const
let colors = ["red", "green", "blue"];
console.log("\nColors:");
for (const color of colors) {
  console.log(color);
}

// for...of with string iteration (each character)
let text = "abc";
console.log("\nString characters:");
for (let char of text) {
  console.log(char);
}

// for...of with break and continue
let values = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
console.log("\nWith break/continue (skip even, break at 7):");
for (let val of values) {
  if (val % 2 === 0) {
    continue;
  }
  if (val === 7) {
    break;
  }
  console.log(val);
}

// Nested for...of loops
let matrix = [
  [1, 2],
  [3, 4],
  [5, 6],
];
console.log("\nNested loops:");
for (let row of matrix) {
  for (let cell of row) {
    console.log(cell);
  }
}

// for...of with mixed array types
let mixed = [1, "hello", true, null];
console.log("\nMixed types:");
for (let item of mixed) {
  console.log(item);
}

// Using array index in for...of (should work with existing features)
let fruits = ["apple", "banana", "cherry"];
console.log("\nFruits with manual indexing:");
let index = 0;
for (let fruit of fruits) {
  console.log(index + ": " + fruit);
  index++;
}

("ok");

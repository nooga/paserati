// expect: done
// Test regular for loops with destructuring initializers

// Array destructuring in for loop initializer
for (const [x, y] = [1, 2]; x < 3; ) {
  console.log(x + "," + y);
  break;
}

// Multiple iterations with destructuring
let count = 0;
for (let [a, b] = [10, 20]; count < 2; count++) {
  console.log(a + b);
}

// Object destructuring in for loop initializer
for (const { x, y } = { x: 5, y: 10 }; x < 10; ) {
  console.log(x + "+" + y);
  break;
}

("done");

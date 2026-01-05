// Async/Await and Generators Demo

console.log("=== Async/Await Demo ===\n");

// Promise chaining
const data = await Promise.resolve({ id: "1", name: "Alice" });
console.log("Fetched:", data.name);

// Promise.all with multiple values
const p1 = Promise.resolve(10);
const p2 = Promise.resolve(20);
const p3 = Promise.resolve(30);
const values = await Promise.all([p1, p2, p3]);
console.log("Sum:", values[0] + values[1] + values[2]);

// Generator function
function* fibonacci(n: number): Generator<number, void, undefined> {
  let [a, b] = [0, 1];
  for (let i = 0; i < n; i++) {
    yield a;
    [a, b] = [b, a + b];
  }
}

const fibs: number[] = [];
for (const n of fibonacci(10)) fibs.push(n);
console.log("\nFibonacci(10):", fibs.join(", "));

// Async generator
async function* countUp(max: number): AsyncGenerator<number, void, undefined> {
  for (let i = 1; i <= max; i++) {
    await Promise.resolve(null);
    yield i;
  }
}

console.log("\nAsync generator:");
let sum = 0;
for await (const n of countUp(5)) {
  sum += n;
  console.log("  yielded:", n, "sum:", sum);
}

console.log("\n✓ Complete");

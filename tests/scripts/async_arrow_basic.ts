// Test async arrow functions
// expect: done

// Simple async arrow
const f1 = async () => 42;
const r1 = await f1();
console.log("r1:", r1);

// Async arrow with parameter
const f2 = async (x: number) => x * 2;
const r2 = await f2(21);
console.log("r2:", r2);

// Async arrow with await
const f3 = async (x: number) => {
    const y = await Promise.resolve(x + 10);
    return y * 2;
};
const r3 = await f3(10);
console.log("r3:", r3);

"done";

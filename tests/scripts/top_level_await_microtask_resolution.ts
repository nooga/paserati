// Test top-level await with promise resolved via microtask
// expect: done
let resolveFunc1: (value: number) => void;
let resolveFunc2: (value: string) => void;

const pendingPromise1 = new Promise<number>((resolve) => {
    resolveFunc1 = resolve;
});

const pendingPromise2 = new Promise<string>((resolve) => {
    resolveFunc2 = resolve;
});

// Schedule resolution in microtasks
Promise.resolve(undefined).then(() => {
    resolveFunc1(42);
    return undefined;
});

Promise.resolve(undefined).then(() => {
    resolveFunc2("hello");
    return undefined;
});

// Await the first promise - will drain microtasks until it settles
const result1 = await pendingPromise1;
console.log("Promise resolved:", result1);

// Await the second promise - already resolved by the earlier microtask drain
const result2 = await pendingPromise2;
console.log("Second promise resolved:", result2);

"done";

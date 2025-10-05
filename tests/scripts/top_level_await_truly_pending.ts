// Test top-level await with a promise that becomes pending then resolved
// This should work because the promise is resolved synchronously before await checks it
// expect: 42

let resolveFunc: (value: number) => void;

const pendingPromise = new Promise<number>((resolve) => {
    resolveFunc = resolve;
});

// Resolve it immediately before awaiting
resolveFunc(42);

// Now await it - the promise should already be fulfilled
const result = await pendingPromise;
result;

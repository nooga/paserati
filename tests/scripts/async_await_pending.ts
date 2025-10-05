// expect: undefined
// Test awaiting a pending promise - should return undefined (promise not resolved synchronously)
let resolveFunc: (value: number) => void;

const pendingPromise = new Promise<number>((resolve) => {
    resolveFunc = resolve;
});

async function testPending() {
    const result = await pendingPromise;
    return result + 100;
}

const p = testPending();
resolveFunc(42);

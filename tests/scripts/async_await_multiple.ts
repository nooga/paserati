// expect: Promise { <pending> }
// Test multiple awaits - should return a Promise
async function multipleAwaits() {
    const a = await Promise.resolve(10);
    const b = await Promise.resolve(20);
    const c = await Promise.resolve(30);
    return a + b + c;
}

multipleAwaits();

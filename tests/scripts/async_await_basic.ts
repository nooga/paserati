// expect: <unknown 19>
// Test basic async/await - should return a Promise
async function test() {
    const result = await Promise.resolve(42);
    return result * 2;
}

test();

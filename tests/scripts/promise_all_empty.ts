// expect: Promise { <pending> }
// Test Promise.all with empty array resolves immediately

async function test() {
    const result = await Promise.all([]);
    return result;
}

test();

// expect: Promise { <pending> }
// Test Promise.all with multiple promises

async function test() {
    const p1 = Promise.resolve(1);
    const p2 = Promise.resolve(2);
    const p3 = Promise.resolve(3);

    const result = await Promise.all([p1, p2, p3]);
    return result;
}

test();

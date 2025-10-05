// expect: Promise { <pending> }
// Test Promise.all with mixed values (promises and non-promises)

async function test() {
    const p1 = Promise.resolve(10);
    const p2 = 20;  // Non-promise value
    const p3 = Promise.resolve(30);

    const result = await Promise.all([p1, p2, p3]);
    return result;
}

test();

// expect: Promise { <pending> }
// Test Promise.race resolves with first settled promise

async function test() {
    const p1 = Promise.resolve(1);
    const p2 = Promise.resolve(2);
    const p3 = Promise.resolve(3);

    const result = await Promise.race([p1, p2, p3]);
    return result;
}

test();

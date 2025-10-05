// expect: Promise { <pending> }
// Test Promise.allSettled waits for all promises regardless of outcome

async function test() {
    const p1 = Promise.resolve(1);
    const p2 = Promise.reject('error');
    const p3 = Promise.resolve(3);

    const results = await Promise.allSettled([p1, p2, p3]);
    return results;
}

test();

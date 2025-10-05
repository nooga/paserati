// expect: Promise { <pending> }
// Test Promise.any resolves with first fulfilled promise

async function test() {
    const result = await Promise.any([
        Promise.reject('error1'),
        Promise.resolve(42),
        Promise.reject('error2')
    ]);
    return result;
}

test();

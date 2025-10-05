// expect: Promise { <pending> }
// Test Promise.all rejects if any promise rejects

async function test() {
    const p1 = Promise.resolve(1);
    const p2 = Promise.reject('error');
    const p3 = Promise.resolve(3);

    try {
        const result = await Promise.all([p1, p2, p3]);
        return 'should not reach here';
    } catch (e) {
        return 'caught: ' + e;
    }
}

test();

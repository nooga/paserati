// expect: Promise { <pending> }
// Test Promise.race rejects if first settled promise rejects

async function test() {
    const p1 = Promise.reject('error');
    const p2 = Promise.resolve(2);

    try {
        const result = await Promise.race([p1, p2]);
        return 'should not reach here';
    } catch (e) {
        return 'caught: ' + e;
    }
}

test();

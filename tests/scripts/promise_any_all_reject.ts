// expect: Promise { <pending> }
// Test Promise.any rejects with AggregateError when all promises reject

async function test() {
    try {
        const result = await Promise.any([
            Promise.reject('error1'),
            Promise.reject('error2')
        ]);
        return 'should not reach here';
    } catch (e) {
        return 'caught error';
    }
}

test();

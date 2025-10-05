// expect: Promise { <pending> }
// Test try/catch in async functions

async function test() {
    try {
        throw new Error('test error');
    } catch (e) {
        return 'recovered';
    }
}

test();

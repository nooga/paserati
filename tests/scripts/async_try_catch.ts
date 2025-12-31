// expect: Promise { recovered }
// Test try/catch in async functions
// When async function completes synchronously (no await), promise is resolved immediately

async function test() {
    try {
        throw new Error('test error');
    } catch (e) {
        return 'recovered';
    }
}

test();

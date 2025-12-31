// expect: Promise { 42 }
// Basic async function test - should return a Promise
// When async function completes synchronously (no await), promise is resolved immediately

async function test() {
    return 42;
}

test();

// Test for-await-of with regular arrays (uses Symbol.asyncIterator)
// expect: compiles

async function test() {
    for await (const x of [1, 2, 3]) {
        const y = x + 1; // x should be number
    }
}

'compiles';

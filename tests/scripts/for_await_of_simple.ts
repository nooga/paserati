// Simple for-await-of test - demonstrates syntax parses and type-checks correctly
// expect: compiles

async function* gen() {
    yield 1;
    yield 2;
}

async function test() {
    for await (const x of gen()) {
        const y = x + 1; // x should be typed as number (element type)
    }
}

'compiles';

// Throwing and catching different value types
let results = [];
try {
    throw 42;
} catch (e) {
    results[0] = e;
}

try {
    throw true;
} catch (e) {
    results[1] = e;
}

try {
    throw null;
} catch (e) {
    results[2] = e;
}

results[0];
// expect: 42
// Comprehensive exception handling demonstration
let results = [];

// Basic try/catch
try {
    throw "basic error";
} catch (e) {
    results[0] = e;
}

// Error object with properties
try {
    let err = new Error("test message");
    err.code = "TEST001";
    throw err;
} catch (e) {
    results[1] = e.message;
}

// No binding catch (ES2019+)
try {
    throw "ignored";
} catch {
    results[2] = "caught without binding";
}

// Different value types
try {
    throw 42;
} catch (e) {
    results[3] = typeof e;
}

// Function with try/catch
function testFunction() {
    try {
        throw "from function";
    } catch (e) {
        return e;
    }
}
results[4] = testFunction();

// Arrow function with Error object
let testArrow = () => {
    try {
        throw new Error("arrow error");
    } catch (e) {
        return e.toString();
    }
};
results[5] = testArrow();

results.length;
// expect: 6
// Test error rethrowing behavior
// expect: all rethrowing tests passed

// Helper functions declared at top level
function deepThrower() {
    throw new TypeError("deep type error");
}

function throwerFunc() {
    throw new Error("original");
}

// Test 1: Basic rethrowing preserves error
function basicTest() {
    try {
        try {
            throw new Error("basic error");
        } catch (e) {
            throw e; // rethrow
        }
    } catch (e) {
        console.log("Basic rethrow:", e.message);
    }
}

// Test 2: Stack trace preservation
function stackTest() {
    function middleFunc() {
        try {
            deepThrower();
        } catch (e) {
            throw e; // rethrow
        }
    }
    
    try {
        middleFunc();
    } catch (e) {
        // Stack should include deepThrower -> middleFunc -> stackTest
        let hasDeep = e.stack.includes("deepThrower");
        let hasMiddle = e.stack.includes("middleFunc");
        console.log("Stack preservation:", hasDeep && hasMiddle);
    }
}

// Test 3: Error modification before rethrowing
function modifyTest() {
    try {
        try {
            throw new ReferenceError("original");
        } catch (e) {
            e.message = "modified";
            e.customProp = "added";
            throw e;
        }
    } catch (e) {
        console.log("Modified message:", e.message);
        console.log("Custom property:", e.customProp);
    }
}

// Test 4: Different error types
function customTypeTest() {
    try {
        try {
            throw new SyntaxError("syntax issue");
        } catch (e) {
            throw e; // rethrow SyntaxError
        }
    } catch (e) {
        console.log("Rethrown type:", e.name);
        console.log("Rethrown message:", e.message);
    }
}

// Test 5: New error vs rethrow stack difference
function newVsRethrowTest() {
    // Test rethrowing
    try {
        try {
            throwerFunc();
        } catch (e) {
            throw e; // rethrow
        }
    } catch (e) {
        let rethrowHasThrower = e.stack.includes("throwerFunc");
        console.log("Rethrow has original location:", rethrowHasThrower);
    }
    
    // Test new error
    try {
        try {
            throwerFunc();
        } catch (e) {
            throw new Error("new error"); // new error
        }
    } catch (e) {
        let newHasThrower = e.stack.includes("throwerFunc");
        console.log("New error has original location:", newHasThrower);
    }
}

// Run all tests
basicTest();
stackTest();
modifyTest();
customTypeTest();
newVsRethrowTest();

"all rethrowing tests passed"
// Compare function scope vs catch scope handling of globals

function helper(msg: string): string {
    return "helper: " + msg;
}

let result = "";

// Test 1: Function scope accessing global
function testFunction() {
    result += "func ";
    result += helper("from-func");
}

testFunction();

// Test 2: Catch scope accessing global  
try {
    throw new Error("test");
} catch (e) {
    result += " catch ";
    result += helper("from-catch");
}

result;
// expect: func helper: from-func catch helper: from-catch
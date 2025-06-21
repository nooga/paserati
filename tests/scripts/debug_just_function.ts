// Test just function scope accessing global

function helper(msg: string): string {
    return "helper: " + msg;
}

function testFunction() {
    return helper("test-modified");
}

testFunction();
// expect: helper: test-modified
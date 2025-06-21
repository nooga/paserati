// Minimal test to reproduce the closure scope issue

function helper(msg: string): string {
    return "helper: " + msg;
}

let result = "start ";

try {
    result += "try ";
    throw new Error("test");
} catch (e) {
    result += "catch ";
    result += helper("test");
}

result;
// expect: start try catch helper: test
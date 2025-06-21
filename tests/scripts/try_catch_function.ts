// Try-catch inside function
function testFunction(): string {
    try {
        throw "function error";
    } catch (e) {
        return e;
    }
}
testFunction();
// expect: function error
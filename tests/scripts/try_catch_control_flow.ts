// Try-catch control flow - code after try block
let step1Executed = false;
let step2Executed = false;
let step3Executed = false;
try {
    step1Executed = true;
    throw "error";
    step2Executed = true; // should not execute
} catch (e) {
    step3Executed = true;
}
step1Executed && !step2Executed && step3Executed;
// expect: true
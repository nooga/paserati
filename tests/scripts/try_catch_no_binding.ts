// Try-catch without catch parameter binding (ES2019+)
let caught = false;
try {
    throw "some error";
} catch {
    caught = true;
}
caught;
// expect: true
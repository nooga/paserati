// Test destructuring without defaults
function test(param) {
    let [a] = param;
    return a;
}

let r1 = test(["PROVIDED"]);
r1;
// expect: PROVIDED
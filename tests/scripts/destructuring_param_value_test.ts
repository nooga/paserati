// Test if parameter value is passed correctly
function test(param) {
    let [a = "DEFAULT"] = param;
    return a;
}

let r1 = test(["PROVIDED"]);
r1;
// expect: PROVIDED
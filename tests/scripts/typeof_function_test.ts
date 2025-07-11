// expect: typeof function test

// Test typeof within a function where variables are already declared

let globalVar = "hello";

function testTypeof() {
    // This should work because globalVar is declared before the function
    let test: typeof globalVar = "world";
    return test;
}

testTypeof();

"typeof function test";
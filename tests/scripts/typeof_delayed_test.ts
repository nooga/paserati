// expect: typeof delayed test

// Test typeof where the variable is definitely declared first

let myVar = "hello";

// Do some other operations first to ensure myVar is processed
let other = myVar + " world";

// Now try typeof
function useTypeof() {
    let test: typeof myVar = "test";
    return test;
}

useTypeof();

"typeof delayed test";
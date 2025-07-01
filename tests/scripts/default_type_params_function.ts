// Test default type parameters in functions - should now work

function test<T = string>() {
    return "hello";
}

// expect: null
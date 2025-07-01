// Test default type parameters in arrow functions - should now work

const test = <T = string>() => {
    return "hello";
};

// expect: null
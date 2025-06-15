// Test delete operator error cases

// Cannot delete variables
let x = 5;
delete x; // expect_compile_error: delete cannot be applied to variables
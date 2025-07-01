// Test manual type import simulation
// This simulates imported types by manually setting up the environment

// Manually imported interface (simulated)
let person: TestInterface = { name: "John", age: 30 };

("manual type import works");

// expect_compile_error: unknown type name

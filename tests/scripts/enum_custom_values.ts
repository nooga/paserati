// expect: custom values enum test

// Test numeric enum with custom values and auto-increment

enum Status {
    Inactive = 0,
    Active = 1,
    Pending = 5,
    Completed    // Should be 6 (5 + 1)
}

function test() {
    // Test explicit values
    if (Status.Inactive !== 0) return "Inactive should be 0";
    if (Status.Active !== 1) return "Active should be 1";
    if (Status.Pending !== 5) return "Pending should be 5";
    
    // Test auto-increment after custom value
    if (Status.Completed !== 6) return "Completed should be 6";
    
    // Test reverse mapping
    if (Status[0] !== "Inactive") return "Status[0] should be 'Inactive'";
    if (Status[1] !== "Active") return "Status[1] should be 'Active'";
    if (Status[5] !== "Pending") return "Status[5] should be 'Pending'";
    if (Status[6] !== "Completed") return "Status[6] should be 'Completed'";
    
    return "all tests passed";
}

test();

"custom values enum test";
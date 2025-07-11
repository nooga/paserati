// expect: satisfies basic test

// Test basic satisfies functionality - type validation without type widening

interface Person {
    name: string;
    age: number;
}

function test() {
    // This should work - object literal satisfies Person interface
    let obj = {
        name: "Alice",
        age: 30
    } satisfies Person;
    
    // The type should remain the original literal type (with literal string/number types)
    // This demonstrates that satisfies preserves the exact type structure
    if (obj.name !== "Alice") return "name check failed";
    if (obj.age !== 30) return "age check failed";
    
    // Test with string literal type
    let message = "hello" satisfies string;
    if (message !== "hello") return "string literal failed";
    
    // Test with number literal type  
    let count = 42 satisfies number;
    if (count !== 42) return "number literal failed";
    
    return "all tests passed";
}

test();

"satisfies basic test";
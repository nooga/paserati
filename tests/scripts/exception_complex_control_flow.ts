// Test complex control flow within try/catch/finally blocks
// This tests that exception handling works correctly with loops, conditionals, and function calls

function helper(value: number): string {
    if (value > 100) {
        throw new Error("Value too large");
    }
    return "helper: " + value;
}

let result = "";

try {
    // Test loops in try block
    for (let i = 0; i < 3; i++) {
        if (i === 1) {
            continue; // Skip iteration
        }
        result += "try-loop-" + i + " ";
    }
    
    // Test conditional with function call
    let testValue = 50;
    if (testValue < 100) {
        result += helper(testValue) + " ";
    }
    
    // Test nested control flow
    let items = ["a", "b", "c"];
    for (let item of items) {
        if (item === "b") {
            break; // Early exit from loop
        }
        result += "item-" + item + " ";
    }
    
    // This should trigger the exception
    helper(150); // Will throw "Value too large"
    
    result += "should-not-reach ";
    
} catch (e) {
    result += "caught: ";
    
    // Test control flow in catch block
    for (let j = 0; j < 2; j++) {
        if (j === 0) {
            result += "catch-" + j + " ";
            continue;
        }
        result += "catch-" + j + " ";
    }
    
    // Test function call in catch
    result += helper(25) + " ";
    
} finally {
    result += "finally: ";
    
    // Test loops in finally
    let count = 0;
    while (count < 2) {
        result += "finally-" + count + " ";
        count++;
    }
    
    // Test conditional in finally
    if (result.indexOf("caught") >= 0) {
        result += "finally-conditional ";
    }
}

result.trim();
// expect: try-loop-0 try-loop-2 helper: 50 item-a caught: catch-0 catch-1 helper: 25 finally: finally-0 finally-1 finally-conditional
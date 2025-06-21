// Test basic control flow within try/catch/finally blocks

let result = "";
let hasTry = false;
let hasCaught = false;

try {
    // Test loops in try block
    for (let i = 0; i < 3; i++) {
        if (i === 1) {
            continue; // Skip iteration
        }
        result += "try-" + i + " ";
    }
    
    hasTry = true;
    
    // Test conditional
    let testValue = 50;
    if (testValue < 100) {
        result += "condition-ok ";
    }
    
    // Test array iteration with break
    let items = ["a", "b", "c"];
    for (let item of items) {
        if (item === "b") {
            break; // Early exit from loop
        }
        result += "item-" + item + " ";
    }
    
    // Trigger exception
    throw new Error("test error");
    
    result += "should-not-reach ";
    
} catch (e) {
    result += "caught ";
    hasCaught = true;
    
    // Test control flow in catch block
    for (let j = 0; j < 2; j++) {
        if (j === 0) {
            result += "catch-" + j + " ";
            continue;
        }
        result += "catch-" + j + " ";
    }
    
    // Test conditional in catch
    if (hasTry) {
        result += "catch-condition ";
    }
    
} finally {
    result += "finally ";
    
    // Test loops in finally
    let count = 0;
    while (count < 2) {
        result += "finally-" + count + " ";
        count++;
    }
    
    // Test conditional in finally
    if (hasCaught) {
        result += "finally-condition";
    }
}

result;
// expect: try-0 try-2 condition-ok item-a caught catch-0 catch-1 catch-condition finally finally-0 finally-1 finally-condition
// Test arguments.hasOwnProperty functionality
function testArguments() {
    console.log("arguments type:", typeof arguments);
    console.log("arguments.hasOwnProperty exists:", typeof arguments.hasOwnProperty);
    
    if (typeof arguments.hasOwnProperty === 'function') {
        console.log("arguments.hasOwnProperty('length'):", arguments.hasOwnProperty("length"));
        console.log("arguments.hasOwnProperty('callee'):", arguments.hasOwnProperty("callee"));
        console.log("arguments.hasOwnProperty('nonexistent'):", arguments.hasOwnProperty("nonexistent"));
    }
    
    return arguments.hasOwnProperty("callee");
}

let result = testArguments();
console.log("Final result:", result);
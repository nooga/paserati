// expect: true

// Test in operator with more complex scenario
function createObject() {
    return { 
        method: function() { return "hello"; },
        value: 42
    };
}

let obj = createObject();
"method" in obj;
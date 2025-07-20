// Test different forms of optional chaining

let obj = {
    method: () => "hello",
    prop: "value"
};

// Test 1: Current supported form - property access
console.log("Property access:", obj?.prop);

// Test 2: Optional call - obj?.method()
console.log("Optional call:", obj?.method?.());

// Test 3: Optional computed access - obj?.["prop"]
console.log("Optional computed:", obj?.["prop"]);

// Test 4: Array access
let arr = [1, 2, 3];
console.log("Array access:", arr?.[0]);

"done";
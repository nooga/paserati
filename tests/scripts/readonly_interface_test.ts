// Test Readonly<T> with object type (interface not fully implemented yet)
// TODO: Implement interface syntax properly

function processUser(user: Readonly<any>) {
    console.log(user.name);
    console.log(user.age);
    return user.name; // Return for testing
}

let user = { name: "Alice", age: 30 };
processUser(user); // Final expression

// expect: Alice
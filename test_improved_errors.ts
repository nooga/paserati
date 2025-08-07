// Test cases for improved VM error messages

// Case 1: Simple function call error
let notFunc: any = 42;
notFunc();

// Case 2: Method call error  
let obj: any = { prop: 123 };
obj.prop();

// Case 3: Constructor call error
let notConstructor: any = "hello";
new notConstructor();
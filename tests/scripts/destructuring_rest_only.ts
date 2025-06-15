// Test rest-only destructuring
let [...everything] = [100, 200, 300];
everything[1];
// expect: 200
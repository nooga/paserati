// Comprehensive Function.prototype.bind() test
// expect: all bind tests completed

console.log("=== Testing Function.prototype.bind() ===");

// Test 1: User-defined function binding
function greet(greeting, name) {
    return greeting + ' ' + name + ', I am ' + this.name;
}

let person = { name: 'Alice' };
let boundGreet = greet.bind(person, 'Hello');
console.log("1. User function bind:", boundGreet('Bob'));

// Test 2: Partial application
function add(a, b, c) {
    return a + b + c;
}

let addTen = add.bind(null, 10);
console.log("2. Partial application:", addTen(5, 3));

// Test 3: Method binding
let obj = { 
    name: 'TestObj', 
    getName: function() { return this.name; } 
};

let boundGetName = obj.getName.bind({ name: 'BoundObj' });
console.log("3. Method binding:", boundGetName());

// Test 4: Native function binding
let arr = [1, 2, 3];
let boundPush = arr.push.bind(arr);
boundPush(4, 5);
console.log("4. Native function bind - array length:", arr.length);

// Test 5: Binding a bound function
function multiply(a, b, c) {
    return a * b * c;
}

let multiplyBy2 = multiply.bind(null, 2);
let multiplyBy2And3 = multiplyBy2.bind(null, 3);
console.log("5. Binding bound function:", multiplyBy2And3(4));

// Test 6: Arrow function in object (should work with bind)
let calculator = {
    base: 10,
    add: function(x) { return this.base + x; }
};

let boundAdd = calculator.add.bind({ base: 100 });
console.log("6. Object method bind:", boundAdd(5));

// Test 7: Multiple partial args
function sum(a, b, c, d) {
    return a + b + c + d;
}

let partialSum = sum.bind(null, 1, 2);
console.log("7. Multiple partial args:", partialSum(3, 4));

console.log("=== All bind tests completed ===");

"all bind tests completed"
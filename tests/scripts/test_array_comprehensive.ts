let numbers = [1, 2, 3, 4, 5];

function double(x: number): number {
    return x * 2;
}

function isEven(x: number): boolean {
    return x % 2 === 0;
}

function isGreaterThan3(x: number): boolean {
    return x > 3;
}

// Test various array methods
console.log("Original array:", numbers);

// Test map
let doubled = numbers.map(double);
console.log("Doubled (map):", doubled);

// Test filter
let evens = numbers.filter(isEven);
console.log("Even numbers (filter):", evens);

// Test forEach
console.log("forEach output:");
numbers.forEach(function(x: number) {
    console.log("  Item:", x);
});

// Test every
let allGreaterThan0 = numbers.every(function(x: number): boolean { return x > 0; });
console.log("All > 0 (every):", allGreaterThan0);

// Test some
let someGreaterThan3 = numbers.some(isGreaterThan3);
console.log("Some > 3 (some):", someGreaterThan3);

// Test find
let firstEven = numbers.find(isEven);
console.log("First even (find):", firstEven);

// Test findIndex
let firstEvenIndex = numbers.findIndex(isEven);
console.log("First even index (findIndex):", firstEvenIndex);
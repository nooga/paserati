let numbers = [1, 2, 3, 4, 5];

function double(x: number): number {
    return x * 2;
}

// Test Array.prototype.map with user-defined function
console.log("Original array:", numbers);
let doubled = numbers.map(double);
console.log("Doubled array:", doubled);

// Test with arrow function (if supported)
let tripled = numbers.map(function(x: number): number {
    return x * 3;
});
console.log("Tripled array:", tripled);
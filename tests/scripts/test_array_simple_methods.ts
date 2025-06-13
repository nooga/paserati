let numbers = [1, 2, 3, 4];

function double(x: number): number {
    return x * 2;
}

function isEven(x: number): boolean {
    return x % 2 === 0;
}

// Test basic methods one by one
console.log("Original:", numbers);

let doubled = numbers.map(double);
console.log("Map result:", doubled);

let evens = numbers.filter(isEven);
console.log("Filter result:", evens);
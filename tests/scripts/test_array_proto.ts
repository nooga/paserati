let arr = [1, 2, 3];

// Test what methods are available on the array
console.log("Array object:", arr);
console.log("Has map property:", 'map' in arr);
console.log("Array prototype:", Object.getPrototypeOf ? Object.getPrototypeOf(arr) : "getPrototypeOf not available");
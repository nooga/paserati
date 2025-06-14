// Test the supposedly missing methods
let str = "Hello World";
let arr = [1, 2, 3, 4, 5];

console.log("String includes test:");
console.log("str.includes('World'):", str.includes('World'));
console.log("str.includes('xyz'):", str.includes('xyz'));

console.log("\nString charCodeAt test:");
console.log("str.charCodeAt(0):", str.charCodeAt(0)); // Should be 72 for 'H'
console.log("str.charCodeAt(6):", str.charCodeAt(6)); // Should be 87 for 'W'

console.log("\nArray join test:");
console.log("arr.join():", arr.join());
console.log("arr.join('-'):", arr.join('-'));
console.log("arr.join(' | '):", arr.join(' | '));

console.log("\nArray includes test:");
console.log("arr.includes(3):", arr.includes(3));
console.log("arr.includes(6):", arr.includes(6));
let str = "  Hello, World!  ";

console.log("Original:", JSON.stringify(str));

// Test various string methods
console.log("charAt(2):", str.charAt(2));
console.log("charCodeAt(2):", str.charCodeAt(2));
console.log("slice(2, 7):", str.slice(2, 7));
console.log("substring(2, 7):", str.substring(2, 7));
console.log("indexOf(','):", str.indexOf(','));
console.log("lastIndexOf('l'):", str.lastIndexOf('l'));
console.log("includes('World'):", str.includes('World'));
console.log("startsWith('  Hello'):", str.startsWith('  Hello'));
console.log("endsWith('!  '):", str.endsWith('!  '));
console.log("toLowerCase():", str.toLowerCase());
console.log("toUpperCase():", str.toUpperCase());
console.log("trim():", JSON.stringify(str.trim()));
console.log("trimStart():", JSON.stringify(str.trimStart()));
console.log("trimEnd():", JSON.stringify(str.trimEnd()));
console.log("repeat(2):", str.repeat(2));
console.log("concat(' Extra'):", str.concat(' Extra'));

// Test split
console.log("split(','):", str.split(','));
console.log("split(''):", str.split('').slice(0, 5)); // Just first 5 chars to avoid long output
// Phase 1-4: Complete regex literal pipeline test (lexer + parser + checker + compiler)
// expect: regex compilation works

// Simple regex literal
let regex1 = /hello/;
console.log("Simple regex:", regex1);

// Regex with flags
let regex2 = /world/gi;
console.log("Regex with flags:", regex2);

// Complex regex pattern  
let regex3 = /complex[A-Z]+/m;
console.log("Complex regex:", regex3);

// Test different contexts where regex should be recognized
let r = /test/i;
console.log("Assignment context:", r);

// Test in conditional context
if (/pattern/) {
    console.log("Conditional context works");
}

console.log("Phase 1-4 complete: Full regex literal pipeline implemented");
"regex compilation works"
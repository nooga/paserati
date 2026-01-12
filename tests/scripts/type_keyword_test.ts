// Test that 'type' keyword is only recognized at statement level for type aliases
// In all expression contexts, 'type' should be a regular identifier

// Type alias declaration still works
type MyType = number;
const typed: MyType = 42;
console.log("type alias:", typed);

// 'type' as variable name with computed property
let type: any = {};
type["key"] = "value";
console.log("type[key]:", type["key"]);

// 'type' with member access
type = { prop: 5 };
console.log("type.prop:", type.prop);

// 'type' in assignment chain
let other;
const chained = type = other = 100;
console.log("type chain:", chained, type, other);

// 'type' with nullish coalescing
type = null;
const result = type ?? "default";
console.log("type ??:", result);

// 'type' in ternary condition
type = true;
console.log("type ternary:", type ? "yes" : "no");

// 'type' as function parameter
function useType(type: any) {
    return type;
}
console.log("type param:", useType(42));

// 'type' in destructuring
const { type: extractedType } = { type: "extracted" };
console.log("destructured:", extractedType);

("type_keyword_passed");

// expect: type_keyword_passed

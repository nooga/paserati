// Debug static readonly with type annotations
class Test {
    static readonly version: string = "1.0";
}

console.log("Parsing test");
"Parsing test"; // Final expression

// expect: Parsing test
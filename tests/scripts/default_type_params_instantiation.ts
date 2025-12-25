// Test type instantiation with default type parameters

type Optional<T = string> = T | undefined;

// Test using default type
let defaultStr: Optional = "hello"; // Should use string default

// Test with explicit type argument  
let explicitNum: Optional<number> = 42;

// Test interface with defaults
interface Container<T = boolean> {
    value: T;
}

let defaultContainer: Container; // Should use boolean default
let explicitContainer: Container<string>; // Should use string

// Test constraint with default
type Constrained<T extends object = {}> = T;

let constrainedDefault: Constrained; // Should use {} default
let constrainedExplicit: Constrained<{name: string}>; // Should use explicit type

// expect: undefined
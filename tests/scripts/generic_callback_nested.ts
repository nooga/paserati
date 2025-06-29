// Test nested generic callbacks with proper type parameter resolution
// expect: 6

// Function that creates a recursive function using a helper
function createRecursive<T extends (n: number) => number>(
    template: (self: T) => T
): T {
    // Create a proper self-referential function
    function recursive(n: number): number {
        return template(recursive as T)(n);
    }
    return recursive as T;
}

type Factorial = (n: number) => number;

// Test with factorial - this tests that type parameters work in nested callbacks
const factorial = createRecursive<Factorial>((self) => (n) => n === 0 ? 1 : n * self(n - 1));

factorial(3);
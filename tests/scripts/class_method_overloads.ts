// expect: 42
// Test method overloading patterns

class Calculator {
    // Method overload signatures
    add(x: number, y: number): number;
    add(x: string, y: string): string;
    
    // Implementation signature
    add(x: number | string, y: number | string): number | string {
        if (typeof x === "number" && typeof y === "number") {
            return x + y;
        } else if (typeof x === "string" && typeof y === "string") {
            return x + y;
        } else {
            throw new Error("Invalid argument types");
        }
    }
    
    // Static method overloads
    static multiply(x: number, y: number): number;
    static multiply(x: string, times: number): string;
    
    // Static implementation
    static multiply(x: number | string, y: number): number | string {
        if (typeof x === "number") {
            return x * y;
        } else {
            return x.repeat(y);
        }
    }
}

// Test different method calls
let calc = new Calculator();
let result1 = calc.add(20, 22);
let result2 = calc.add("Hello", " World");
let result3 = Calculator.multiply(3, 4);
let result4 = Calculator.multiply("Hi", 3);

result1;
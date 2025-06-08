// expect: 15

// Comprehensive test showing various type assertion scenarios
interface Calculator {
    add(a: number, b: number): number;
}

function createCalculator(): unknown {
    return {
        add: function(a: number, b: number): number {
            return a + b;
        }
    };
}

// Type assertion in a realistic scenario
let calc = createCalculator() as Calculator;
let result = calc.add(7, 8);
result;
// Test class export
export class TestClass {
    value: number;
    constructor(val: number) {
        this.value = val;
    }
    getValue(): number {
        return this.value;
    }
}

export interface TestInterface {
    name: string;
    age: number;
}

console.log("Module exports class and interface");
"exported successfully";
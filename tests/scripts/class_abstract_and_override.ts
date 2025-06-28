// expect: Testing abstract classes without inheritance
// Test abstract classes without inheritance (inheritance not yet implemented)

abstract class Shape {
    name: string;
    
    constructor(name: string) {
        this.name = name;
    }
    
    // Abstract method - must be implemented by subclasses
    abstract area(): number;
    abstract perimeter(): number;
    
    // Concrete method
    getName(): string {
        return this.name;
    }
}

class Rectangle {
    private width: number;
    private height: number;
    private name: string;
    
    constructor(width: number, height: number) {
        this.name = "Rectangle";
        this.width = width;
        this.height = height;
    }
    
    // Regular methods (not override since no inheritance)
    area(): number {
        return this.width * this.height;
    }
    
    perimeter(): number {
        return 2 * (this.width + this.height);
    }
    
    describe(): string {
        return `${this.name} area: ${this.area()}`;
    }
}

let rect = new Rectangle(10, 5);

"Testing abstract classes without inheritance";
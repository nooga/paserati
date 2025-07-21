// expect: Circle area: 78.54
// Test abstract classes and methods

abstract class Shape {
    protected name: string;
    
    constructor(name: string) {
        this.name = name;
    }
    
    // Abstract method - must be implemented by subclasses
    abstract area(): number;
    abstract perimeter(): number;
    
    // Concrete method - can be inherited
    getName(): string {
        return this.name;
    }
    
    // Concrete method using abstract method
    describe(): string {
        return `${this.name} area: ${this.area().toFixed(2)}`;
    }
}

class Circle extends Shape {
    private radius: number;
    
    constructor(radius: number) {
        super("Circle");
        this.radius = radius;
    }
    
    // Must implement abstract methods
    area(): number {
        return Math.PI * this.radius * this.radius;
    }
    
    perimeter(): number {
        return 2 * Math.PI * this.radius;
    }
    
    getRadius(): number {
        return this.radius;
    }
}

class Rectangle extends Shape {
    private width: number;
    private height: number;
    
    constructor(width: number, height: number) {
        super("Rectangle");
        this.width = width;
        this.height = height;
    }
    
    area(): number {
        return this.width * this.height;
    }
    
    perimeter(): number {
        return 2 * (this.width + this.height);
    }
}

// Cannot instantiate abstract class
// let shape = new Shape("test"); // Error

let circle = new Circle(5);
circle.describe();
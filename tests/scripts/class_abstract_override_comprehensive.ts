// expect: Comprehensive abstract and override test completed
// Comprehensive test for abstract classes and override keyword

// 1. Abstract class with abstract methods
abstract class Vehicle {
    protected brand: string;
    
    constructor(brand: string) {
        this.brand = brand;
    }
    
    // Abstract methods that must be implemented by subclasses
    abstract start(): string;
    abstract stop(): string;
    
    // Concrete method that can be overridden
    getBrand(): string {
        return this.brand;
    }
}

// 2. Concrete class extending abstract class
class Car extends Vehicle {
    private model: string;
    
    constructor(brand: string, model: string) {
        super(brand);
        this.model = model;
    }
    
    // Implementing abstract methods (override keyword is optional but recommended)
    override start(): string {
        return `${this.brand} ${this.model} engine started`;
    }
    
    override stop(): string {
        return `${this.brand} ${this.model} engine stopped`;
    }
    
    // Overriding concrete method
    override getBrand(): string {
        return `${this.brand} (car)`;
    }
}

// 3. Test that concrete classes work
let car = new Car("Toyota", "Camry");

// Note: The following would cause compile errors:
// let vehicle = new Vehicle("Generic"); // Error: cannot instantiate abstract class

"Comprehensive abstract and override test completed";
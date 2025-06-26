// expect: "Flying at 100 mph"
// Test classes implementing interfaces

interface Flyable {
    speed: number;
    fly(): string;
    land(): void;
}

interface Swimmable {
    depth: number;
    swim(): string;
    surface(): void;
}

interface Named {
    name: string;
    getName(): string;
}

// Class implementing single interface
class Bird implements Flyable, Named {
    name: string;
    speed: number;
    
    constructor(name: string, speed: number) {
        this.name = name;
        this.speed = speed;
    }
    
    fly(): string {
        return `Flying at ${this.speed} mph`;
    }
    
    land(): void {
        // Landing logic
    }
    
    getName(): string {
        return this.name;
    }
}

// Class implementing multiple interfaces
class Duck implements Flyable, Swimmable, Named {
    name: string;
    speed: number;
    depth: number;
    
    constructor(name: string) {
        this.name = name;
        this.speed = 25;
        this.depth = 10;
    }
    
    // Implement Flyable
    fly(): string {
        return `${this.name} flying at ${this.speed} mph`;
    }
    
    land(): void {
        // Landing on water
    }
    
    // Implement Swimmable
    swim(): string {
        return `${this.name} swimming at depth ${this.depth} feet`;
    }
    
    surface(): void {
        this.depth = 0;
    }
    
    // Implement Named
    getName(): string {
        return this.name;
    }
}

// Class with interface inheritance
class Airplane implements Flyable {
    speed: number;
    private pilot: string;
    
    constructor(pilot: string, speed: number) {
        this.pilot = pilot;
        this.speed = speed;
    }
    
    fly(): string {
        return `Airplane piloted by ${this.pilot} flying at ${this.speed} mph`;
    }
    
    land(): void {
        // Landing at airport
    }
}

let eagle = new Bird("Eagle", 100);
eagle.fly();
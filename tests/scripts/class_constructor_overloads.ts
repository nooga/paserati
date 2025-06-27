// expect: Point at (5, 10)
// Test constructor overloading patterns

class Point {
    x: number;
    y: number;
    
    // Constructor overload signatures (TypeScript feature)
    constructor(x: number, y: number);
    constructor(coordinates: { x: number; y: number });
    constructor(copyFrom: Point);
    
    // Implementation signature
    constructor(xOrObject: number | { x: number; y: number } | Point, y?: number) {
        if (typeof xOrObject === "number" && typeof y === "number") {
            // Point(x, y)
            this.x = xOrObject;
            this.y = y;
        } else if (typeof xOrObject === "object" && "x" in xOrObject && "y" in xOrObject) {
            // Point({x, y}) or Point(otherPoint)
            this.x = xOrObject.x;
            this.y = xOrObject.y;
        } else {
            throw new Error("Invalid constructor arguments");
        }
    }
    
    toString(): string {
        return `Point at (${this.x}, ${this.y})`;
    }
    
    static origin(): Point {
        return new Point(0, 0);
    }
    
    static fromCoords(coords: { x: number; y: number }): Point {
        return new Point(coords);
    }
}

// Test different constructor calls
let p1 = new Point(5, 10);
let p2 = new Point({ x: 3, y: 4 });
let p3 = new Point(p1);
let origin = Point.origin();

p1.toString();
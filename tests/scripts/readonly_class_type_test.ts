// Test Readonly<T> with class instance type
class Point {
    x = 10;
    y = 20;
}

let point: Readonly<Point> = new Point();
console.log(point.x); // Should work - reading
console.log(point.y); // Should work - reading
point.x; // Final expression

// expect: 10
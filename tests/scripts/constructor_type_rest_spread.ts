// expect: true
class Box {
    constructor(x: number, y: number, ...labels: string[]) {}
}

let labels = ["a", "b"];
let holder: { make: { new (x: number, y: number, ...labels: string[]): Box } } = { make: Box };
let made = new holder.make(1, 2, ...labels, "c");

made !== undefined;

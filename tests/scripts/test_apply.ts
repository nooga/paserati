function add(x: number, y: number): number {
    return x + y;
}

// Test Function.prototype.apply
console.log("Testing Function.prototype.apply:");
console.log("add.apply(null, [3, 4]):", add.apply(null, [3, 4]));
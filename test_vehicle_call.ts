// Minimal test for Vehicle.call issue
function Vehicle(wheels: number) {
  console.log("Vehicle constructor called with wheels:", wheels);
  this.wheels = wheels;
}

console.log("Vehicle type:", typeof Vehicle);
console.log("Vehicle.call type:", typeof Vehicle.call);

// Try calling Vehicle.call
let obj: any = {};
console.log("Calling Vehicle.call(obj, 4)");
Vehicle.call(obj, 4);
console.log("obj.wheels:", obj.wheels);
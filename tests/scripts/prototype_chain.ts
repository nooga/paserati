// expect: true
// Test prototype chain traversal
function Vehicle(wheels: number) {
  this.wheels = wheels;
}

Vehicle.prototype.move = function () {
  return "Moving on " + this.wheels + " wheels";
};

function Car(brand: string) {
  // Call parent constructor
  Vehicle.call(this, 4);
  this.brand = brand;
}

// Set up prototype chain (manual for now, until we have Object.create)
Car.prototype = new Vehicle(0);
Car.prototype.constructor = Car;

Car.prototype.drive = function () {
  return "Driving " + this.brand;
};

let myCar = new Car("Toyota");

// Test own properties
console.log(myCar.brand); // expect: Toyota
console.log(myCar.wheels); // expect: 4

// Test methods from immediate prototype
console.log(myCar.drive()); // expect: Driving Toyota

// Test methods from parent prototype
console.log(myCar.move()); // expect: Moving on 4 wheels

// Test instanceof
console.log(myCar instanceof Car); // expect: true
console.log(myCar instanceof Vehicle); // expect: true

// Test property existence
console.log("brand" in myCar); // expect: true
console.log("wheels" in myCar); // expect: true
console.log("drive" in myCar); // expect: true
console.log("move" in myCar); // expect: true

true;

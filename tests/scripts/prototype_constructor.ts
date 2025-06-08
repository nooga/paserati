// expect: true
// Test constructor property and prototype relationships
function Animal(species: string) {
  this.species = species;
}

// Check that prototype has constructor property
console.log(Animal.prototype.constructor === Animal); // true

// Create instance
let cat = new Animal("feline");

// Instance should inherit constructor from prototype
console.log(cat.constructor === Animal); // true

// Multiple instances share the same prototype
let dog = new Animal("canine");
console.log(Object.getPrototypeOf(cat) === Object.getPrototypeOf(dog)); // expect_runtime_error: Object.getPrototypeOf is not a function

// For now, we can test that both have the same constructor
console.log(cat.constructor === dog.constructor); // true (final expression)

cat.constructor === dog.constructor;

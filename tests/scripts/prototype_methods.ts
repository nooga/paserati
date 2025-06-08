// expect: 19
// Test prototype methods and 'this' binding
function Counter(initial: number) {
  this.count = initial;
}

Counter.prototype.increment = function () {
  this.count++;
  return this.count;
};

Counter.prototype.decrement = function () {
  this.count--;
  return this.count;
};

Counter.prototype.reset = function () {
  this.count = 0;
};

// Create instances
let c1 = new Counter(10);
let c2 = new Counter(20);

// Test that each instance has its own state
console.log(c1.count); // 10
console.log(c2.count); // 20

// Test method calls modify correct instance
console.log(c1.increment()); // 11
console.log(c1.increment()); // 12
console.log(c2.decrement()); // 19

// Verify instances are independent
console.log(c1.count); // 12
console.log(c2.count); // 19

// Test method sharing
console.log(c1.increment === c2.increment); // true

// Test reset
c1.reset();
console.log(c1.count); // 0
console.log(c2.count); // 19 (final expression)

c2.count;

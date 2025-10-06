// Test that Proxy trap calls don't break class inheritance
// expect: Buddy
// This is a regression test for a bug where calling Proxy traps would
// leave the sentinel frame flag set, causing subsequent class constructors
// to be incorrectly treated as sentinel frames and hang the VM.

const proxy = new Proxy({ x: 1 }, {
  get(t, k) {
    return Reflect.get(t, k);
  }
});

// Call the trap (this used to leave sentinel frame flag set)
const val = proxy.x;

// Define a class hierarchy
class Animal {
  constructor(public name: string) {}
}

class Dog extends Animal {
  constructor(name: string) {
    super(name);
  }
}

// Create instance - this used to hang
const dog = new Dog("Buddy");
dog.name;

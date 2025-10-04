// Simplest test for # private fields
class C {
  #x: number = 42;
}

const c = new C();
console.log(c.#x);  // Should error: private field accessed outside class

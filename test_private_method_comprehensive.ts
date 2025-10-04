// Test 1: Instance private method
class Test1 {
  #privateMethod() { return "private"; }
  
  callIt() { return this.#privateMethod(); }
  refIt() { return this.#privateMethod; }
}

const t1 = new Test1();
console.log("Test1 call:", t1.callIt());
const fn1 = t1.refIt();
console.log("Test1 ref:", fn1());

// Test 2: Static private method
class Test2 {
  static #staticPrivate() { return "static private"; }
  
  static callIt() { return this.#staticPrivate(); }
  static refIt() { return this.#staticPrivate; }
}

console.log("Test2 static call:", Test2.callIt());
const fn2 = Test2.refIt();
console.log("Test2 static ref:", fn2());

// Test 3: Private generator method
class Test3 {
  *#genMethod() { yield 1; yield 2; }
  
  getGen() { return this.#genMethod; }
}

const t3 = new Test3();
const gen = t3.getGen();
const g = gen();
console.log("Test3 gen:", g.next().value, g.next().value);

// Test 4: Shared across instances
class Test4 {
  #method() { return "shared"; }
  getMethod() { return this.#method; }
}

const t4a = new Test4();
const t4b = new Test4();
console.log("Test4 same function:", t4a.getMethod() === t4b.getMethod());

console.log("All tests passed!");

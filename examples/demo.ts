// Paserati Language Feature Demo
// Showcases: Proxies, Reflect, RegExp, Maps, Sets, Classes, Generics

console.log("╔═══════════════════════════════════════╗");
console.log("║     Paserati Feature Showcase         ║");
console.log("╚═══════════════════════════════════════╝\n");

// ============================================================================
// Demo 1: Proxies and Reflect
// ============================================================================

console.log("--- Proxy and Reflect Demo ---\n");

const target = { name: "Alice", age: 30 };

const handler = {
  get(obj, prop) {
    console.log(`[Proxy] Getting property: ${prop}`);
    return Reflect.get(obj, prop);
  },
  set(obj, prop, value) {
    console.log(`[Proxy] Setting ${prop} = ${value}`);
    return Reflect.set(obj, prop, value);
  },
};

const proxy = new Proxy(target, handler);

console.log("Name:", proxy.name);
proxy.age = 31;
console.log("Age:", proxy.age);

console.log("\nReflect API:");
console.log("Has 'name':", Reflect.has(target, "name"));
console.log("Own keys:", Reflect.ownKeys(target));

// ============================================================================
// Demo 2: Regular Expressions
// ============================================================================

console.log("\n--- Regular Expression Demo ---\n");

const emailPattern = /^[a-z]+@[a-z]+\.[a-z]+$/;
const email1 = "alice@example.com";
const email2 = "invalid.email";

console.log(`"${email1}" is valid:`, emailPattern.test(email1));
console.log(`"${email2}" is valid:`, emailPattern.test(email2));

const text = "The quick brown fox jumps over the lazy dog";
const pattern = /\b\w{5}\b/g;
const matches: string[] = [];
let match = pattern.exec(text);
while (match !== null) {
  const m = match as string[];
  matches.push(m[0]);
  match = pattern.exec(text);
}
console.log("5-letter words:", matches.join(", "));

// ============================================================================
// Demo 3: Map and Set Collections
// ============================================================================

console.log("\n--- Map and Set Demo ---\n");

const scores = new Map<string, number>();
scores.set("Alice", 95);
scores.set("Bob", 87);
scores.set("Charlie", 92);

console.log("Alice's score:", scores.get("Alice"));
console.log("Has Bob:", scores.has("Bob"));
console.log("Map size:", scores.size);

const uniqueNumbers = new Set<number>();
uniqueNumbers.add(1);
uniqueNumbers.add(2);
uniqueNumbers.add(2); // duplicate
uniqueNumbers.add(3);

console.log("Set size:", uniqueNumbers.size);
console.log("Has 2:", uniqueNumbers.has(2));
console.log("Has 5:", uniqueNumbers.has(5));

// ============================================================================
// Demo 4: Classes with Generics
// ============================================================================

console.log("\n--- Generic Class Demo ---\n");

class Stack<T> {
  private items: T[] = [];

  push(item: T) {
    this.items.push(item);
  }

  pop(): T {
    return this.items.pop() as T;
  }

  peek(): T {
    return this.items[this.items.length - 1];
  }

  size(): number {
    return this.items.length;
  }
}

const numberStack = new Stack<number>();
numberStack.push(10);
numberStack.push(20);
numberStack.push(30);

console.log("Stack size:", numberStack.size());
console.log("Peek:", numberStack.peek());
console.log("Pop:", numberStack.pop());
console.log("New size:", numberStack.size());

const stringStack = new Stack<string>();
stringStack.push("hello");
stringStack.push("world");
console.log("String stack pop:", stringStack.pop());

// ============================================================================
// Demo 5: Class Inheritance
// ============================================================================

console.log("\n--- Class Inheritance Demo ---\n");

class Animal {
  constructor(public name: string) {}

  speak() {
    console.log(`${this.name} makes a sound`);
  }
}

class Dog extends Animal {
  constructor(name: string, public breed: string) {
    super(name);
  }

  speak() {
    console.log(`${this.name} barks!`);
  }

  fetch() {
    console.log(`${this.name} fetches the ball`);
  }
}

const dog = new Dog("Buddy", "Golden Retriever");
dog.speak();
dog.fetch();
console.log("Breed:", dog.breed);

// ============================================================================
// Demo 6: Arrow Functions and Closures
// ============================================================================

console.log("\n--- Closures Demo ---\n");

function createCounter(start: number) {
  let count = start;

  return {
    increment: () => ++count,
    decrement: () => --count,
    getValue: () => count,
  };
}

const counter = createCounter(10);
console.log("Initial:", counter.getValue());
counter.increment();
counter.increment();
console.log("After 2 increments:", counter.getValue());
counter.decrement();
console.log("After 1 decrement:", counter.getValue());

// ============================================================================
// Demo 7: Type Guards and Union Types
// ============================================================================

console.log("\n--- Type Guards Demo ---\n");

function processValue(value: string | number) {
  if (typeof value === "string") {
    console.log("String length:", value.length);
  } else {
    const n = value as number;
    console.log("Number doubled:", n * 2);
  }
}

processValue("hello");
processValue(42);

// ============================================================================
// Demo 8: Destructuring
// ============================================================================

console.log("\n--- Destructuring Demo ---\n");

const person = { firstName: "John", lastName: "Doe", age: 25 };
const { firstName, lastName } = person;
console.log(`Name: ${firstName} ${lastName}`);

const numbers = [1, 2, 3, 4, 5];
const [first, second, ...rest] = numbers;
console.log("First:", first);
console.log("Second:", second);
console.log("Rest:", rest);

// ============================================================================
// Summary
// ============================================================================

console.log("\n╔═══════════════════════════════════════╗");
console.log("║        Demo Complete!                 ║");
console.log("╚═══════════════════════════════════════╝\n");

console.log("✨ Featured:");
console.log("  • Proxy and Reflect API");
console.log("  • Regular expressions");
console.log("  • Map and Set collections");
console.log("  • Generic classes");
console.log("  • Class inheritance with super");
console.log("  • Arrow functions and closures");
console.log("  • Type guards");
console.log("  • Destructuring");
console.log("\nAll running natively in Paserati! 🚀\n");

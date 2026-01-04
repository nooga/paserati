// Reactive Signals System - Type-Safe Reactivity in Pure TypeScript
// expect: Name: Alice, Age: 31, isAdult: true, Profile: Alice (30 years old)

// === Generic Signal Class with Fluent API ===
class Signal<T> {
  private value: T;
  private subscribers: Array<(value: T) => void>;

  constructor(initial: T) {
    this.value = initial;
    this.subscribers = [];
  }

  read(): T {
    return this.value;
  }

  write(newValue: T): Signal<T> {
    this.value = newValue;
    for (let i = 0; i < this.subscribers.length; i++) {
      this.subscribers[i](newValue);
    }
    return this;
  }

  // Generic method - returns new Signal with transformed type U
  map<U>(fn: (x: T) => U): Signal<U> {
    const mapped = new Signal<U>(fn(this.value));
    this.subscribe((v: T) => mapped.write(fn(v)));
    return mapped;
  }

  subscribe(fn: (value: T) => void): Signal<T> {
    this.subscribers.push(fn);
    return this;
  }
}

// === Generic computed signal factory ===
function computed<T>(computation: () => T): Signal<T> {
  return new Signal<T>(computation());
}

// === Create reactive state with type inference ===
const name = new Signal<string>("Alice");
const age = new Signal<number>(30);

// === Derived signals - type U inferred from transformation ===
const isAdult = age.map((a: number) => a >= 18);
const profile = computed(() => name.read() + " (" + age.read() + " years old)");

// === Reactive update ===
age.write(31);

// === Output ===
"Name: " + name.read() + ", Age: " + age.read() + ", isAdult: " + isAdult.read() + ", Profile: " + profile.read();

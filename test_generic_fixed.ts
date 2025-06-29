// Test successful generic features (parser-compatible version)

// 1. Generic constructor inference
class Container<T> {
  value: T;
  
  constructor(value: T) {
    this.value = value;
  }
  
  transform<U>(fn: (x: T) => U): Container<U> {
    return new Container(fn(this.value));
  }
}

const c1 = new Container(42);
const c2 = c1.transform(x => x * 2);
const c3 = c2.transform(x => x > 50);

console.log("Container value:", c3.value);

// 2. Method chaining with generics  
class Chain<T> {
  items: T[];
  
  constructor(items: T[]) {
    this.items = items;
  }
  
  map<U>(fn: (x: T) => U): Chain<U> {
    const mapped: U[] = [];
    for (let i = 0; i < this.items.length; i++) {
      mapped.push(fn(this.items[i]));
    }
    return new Chain(mapped);
  }
  
  filter(pred: (x: T) => boolean): Chain<T> {
    const filtered: T[] = [];
    for (let i = 0; i < this.items.length; i++) {
      if (pred(this.items[i])) {
        filtered.push(this.items[i]);
      }
    }
    return new Chain(filtered);
  }
  
  first(): T | undefined {
    return this.items[0];
  }
}

const chain = new Chain([1, 2, 3, 4, 5])
  .map(x => x * 10)
  .filter(x => x > 25)
  .map(x => ({ value: x, label: "num" }));

const result = chain.first();
console.log("Chain result value:", result ? result.value : "none");

// 3. Nested generic returns
function wrapInArray<T>(x: T): T[] {
  return [x];
}

function wrapInObject<T>(x: T): { data: T } {
  return { data: x };
}

const nested = wrapInObject(wrapInArray(100));
console.log("Nested:", nested.data[0]);

console.log("All generic tests passed!");

// expect: Container value: true
// expect: Chain result value: 30
// expect: Nested: 100
// expect: All generic tests passed!
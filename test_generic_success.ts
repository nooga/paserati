// Test successful generic features

// 1. Generic constructor inference
class Container<T> {
  constructor(public value: T) {}
  
  transform<U>(fn: (x: T) => U): Container<U> {
    return new Container(fn(this.value));
  }
}

const c1 = new Container(42);
const c2 = c1.transform(x => x * 2);
const c3 = c2.transform(x => x > 50);

console.log("Container tests passed");

// 2. Method chaining with generics  
class Chain<T> {
  constructor(private items: T[]) {}
  
  map<U>(fn: (x: T) => U): Chain<U> {
    return new Chain(this.items.map(fn));
  }
  
  filter(pred: (x: T) => boolean): Chain<T> {
    return new Chain(this.items.filter(pred));
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
console.log("Chain result:", result);

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

// expect: Container tests passed
// expect: Chain result: { value: 30, label: num }
// expect: Nested: 100
// expect: All generic tests passed!
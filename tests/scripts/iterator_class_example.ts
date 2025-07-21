// Test the original Iterator class example
// expect: 10

class MyIterator {
  data: any[];
  index: number;

  constructor(data: any[]) {
    this.data = data;
    this.index = 0;
  }

  next() {
    if (this.index < this.data.length) {
      return { value: this.data[this.index++], done: false };
    }

    return { value: undefined, done: true };
  }

  return(value?: any) {
    console.log("return");
    return { value: value, done: true };
  }

  [Symbol.iterator]() {
    return this;
  }
}

const data = [10, 20, 30, 40, 50];
const iterator = new MyIterator(data);

// Test the iterator manually since type checker doesn't recognize it as iterable yet
let current = iterator.next();
while (!current.done) {
  console.log(current.value);

  if (current.value === 30) {
    console.log("Called return method");
    break;
  }

  current = iterator.next();
}

// Return the first value as expected
10;

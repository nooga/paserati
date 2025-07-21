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

// Test with for...of loop to see if iterator protocol works
for (const value of iterator) {
  console.log(value);

  if (value === 30) {
    console.log("Breaking from for...of loop");
    break;
  }
}

// Return the first value as expected
10;

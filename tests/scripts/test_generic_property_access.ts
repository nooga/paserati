// Test property access on generic types after method chaining

interface DataPoint {
  id: string;
  value: number;
}

class Processor<T> {
  private data: T[];

  constructor(data: T[]) {
    this.data = data;
  }

  transform<U>(fn: (item: T) => U): Processor<U> {
    const result = this.data.map(fn);
    return new Processor(result);
  }

  filter(predicate: (item: T) => boolean): Processor<T> {
    return new Processor(this.data.filter(predicate));
  }
}

const data: DataPoint[] = [
  { id: "1", value: 100 }
];

const processor = new Processor(data);

// Transform that adds a new property
const enriched = processor.transform((item) => {
  return {
    ...item,
    doubled: item.value * 2  // Add a new property
  };
});

// This should work - accessing properties on the transformed type
const filtered = enriched.filter((item) => item.doubled > 150);

"Test completed";

// expect: Test completed
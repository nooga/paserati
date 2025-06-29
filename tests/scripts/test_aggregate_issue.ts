// Test aggregate method type inference

class DataProcessor<T> {
  private data: T[];

  constructor(initialData: T[]) {
    this.data = [...initialData];
  }

  aggregate<TResult>(
    initialValue: TResult,
    aggregator: (acc: TResult, current: T) => TResult
  ): TResult {
    return this.data.reduce(aggregator, initialValue);
  }
}

interface Item {
  value: number;
  name: string;
}

const processor = new DataProcessor<Item>([
  { value: 10, name: "a" },
  { value: 20, name: "b" }
]);

// Test aggregate - the acc parameter should be inferred as the initial value type
const result = processor.aggregate(
  { total: 0, count: 0 },
  (acc, item) => ({
    total: acc.total + item.value,  // acc.total should be number
    count: acc.count + 1            // acc.count should be number
  })
);

console.log(result.total, result.count);
`${result.total} ${result.count}`;

// expect: 30 2
// Debug the transform method type inference issue

// Let's create a minimal reproduction of the DataProcessor.transform issue

interface DataPoint<T extends string | number> {
  id: string;
  value: T;
  timestamp: number;
}

type Transformer<TInput, TOutput> = (input: TInput) => TOutput;

class DataProcessor<T> {
  private data: T[];

  constructor(initialData: T[]) {
    this.data = [...initialData];
  }

  // This is the problematic method
  transform<U>(transformer: Transformer<T, U>): DataProcessor<U> {
    const transformed = this.data.map(transformer);
    return new DataProcessor(transformed);
  }
}

// Test data
const salesData: DataPoint<number>[] = [
  { id: "Q1-2024", value: 125000, timestamp: Date.now() },
];

const processor = new DataProcessor(salesData);

// This should work but is giving a type error
const enriched = processor.transform((dataPoint) => {
  const { id, value, timestamp } = dataPoint;
  const [quarter, year] = id.split("-");

  return {
    ...dataPoint,
    quarter,
    year: parseInt(year),
    valueInK: Math.round(value / 1000),
  };
});

console.log("Transform test completed");

// expect: Transform test completed
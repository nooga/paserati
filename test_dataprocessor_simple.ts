// Simplified DataProcessor test

type Transformer<TInput, TOutput> = (input: TInput) => TOutput;

class DataProcessor<T> {
  private data: T[];

  constructor(initialData: T[]) {
    this.data = initialData;
  }

  transform<U>(transformer: Transformer<T, U>): DataProcessor<U> {
    const transformed = this.data.map(transformer);
    return new DataProcessor(transformed); // This should work with type inference
  }
}

const processor = new DataProcessor([1, 2, 3]);
const stringProcessor = processor.transform(x => x.toString());

console.log("Test completed");
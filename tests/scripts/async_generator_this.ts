// Test async generator methods accessing 'this'
// expect: success

class DataStore<T> {
  private items: T[] = [];

  add(item: T): void {
    this.items.push(item);
  }

  async *stream(): AsyncGenerator<T, void, undefined> {
    for (const item of this.items) {
      await Promise.resolve(null);
      yield item;
    }
  }

  count(): number {
    return this.items.length;
  }
}

const store = new DataStore<number>();
store.add(1);
store.add(2);
store.add(3);

// Use async function and await the result
await (async () => {
  let sum = 0;
  for await (const num of store.stream()) {
    sum += num;
  }

  // If we got here with correct sum, 'this' binding worked
  if (sum === 6 && store.count() === 3) {
    return "success";
  } else {
    return "failed";
  }
})();

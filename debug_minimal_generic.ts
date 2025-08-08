class Test<T> {
  data: T[];
  constructor(data: T[]) { this.data = data; }
  filter(fn: (item: T) => boolean): Test<T> {
    return new Test(this.data.filter(fn));
  }
}

const test = new Test([{value: 100}]);
const filtered = test.filter((item) => item.value > 50);
"Test completed";
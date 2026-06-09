// expect: 1
const obj = {
  value<T>(x: T): T {
    return x;
  }
};

1;

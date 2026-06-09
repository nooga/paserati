// expect: 1
interface Pair<T extends U, U extends Date> {
  value(x: T, y: U): string;
}

1;

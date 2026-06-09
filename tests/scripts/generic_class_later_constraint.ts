// expect: 1
class Box<T extends U, U extends Date> {
  value(x: T, y: U): string {
    return "";
  }
}

1;

// expect: 2
class Box<T extends U, U extends Array<number>> {
  value: T;

  constructor(value: T) {
    this.value = value;
  }
}

let box = new Box<Array<number>, Array<number>>([1, 2]);
box.value.length;

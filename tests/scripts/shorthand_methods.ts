// expect: Hello, John! 0

const obj = {
  name: "John",
  age: 30,
  greet(x: number = 0) {
    return `Hello, ${this.name}! ${x}`;
  },
};

obj.greet();

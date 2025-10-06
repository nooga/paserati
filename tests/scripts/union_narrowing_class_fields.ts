// expect: all tests passed
// Test union type narrowing with class fields

class Container {
  value: string | number;

  constructor(v: string | number) {
    this.value = v;
  }

  // Test 1: typeof narrowing
  processValue(): string {
    if (typeof this.value === "string") {
      return this.value.toUpperCase();
    } else {
      return this.value.toString();
    }
  }
}

const c1 = new Container("hello");
const c2 = new Container(42);

if (c1.processValue() !== "HELLO") {
  console.log("Test 1 failed: string narrowing");
}

if (c2.processValue() !== "42") {
  console.log("Test 2 failed: number narrowing");
}

// Test 3: Complex union type
class Message {
  data: string | { text: string };

  constructor(d: string | { text: string }) {
    this.data = d;
  }

  getText(): string {
    if (typeof this.data === "string") {
      return this.data;
    } else {
      return this.data.text;
    }
  }
}

const m1 = new Message("direct");
const m2 = new Message({ text: "wrapped" });

if (m1.getText() !== "direct") {
  console.log("Test 3 failed: string message");
}

if (m2.getText() !== "wrapped") {
  console.log("Test 4 failed: object message");
}

"all tests passed";

// Test file for shorthand method syntax in object type literals
// expect: success

// Object type with shorthand method syntax
type Calculator = {
  add(a: number, b: number): number;
  subtract(a: number, b: number): number;
  multiply(a: number, b: number): number;
  divide?(a: number, b: number): number; // optional method
};

// Implementation that matches the type
let calc: Calculator = {
  add(a: number, b: number): number {
    return a + b;
  },
  subtract(a: number, b: number): number {
    return a - b;
  },
  multiply(a: number, b: number): number {
    return a * b;
  },
  // divide is optional, so we can omit it
};

// Test basic operations
let sum = calc.add(10, 5);
let diff = calc.subtract(10, 5);
let product = calc.multiply(10, 5);

// Object type with mixed syntax
type MixedSyntax = {
  name: string;
  age: number;
  greet(greeting: string): string;
  getName(): string;
  setAge?(newAge: number): void; // optional method
};

let person: MixedSyntax = {
  name: "John",
  age: 30,
  greet(greeting: string): string {
    return greeting + ", " + this.name;
  },
  getName(): string {
    return this.name;
  },
  // setAge is optional, omitted
};

// Test methods
let greeting = person.greet("Hello");
let name = person.getName();

// Object type with methods that have no parameters
type SimpleActions = {
  start(): void;
  stop(): void;
  reset?(): void; // optional
};

let actions: SimpleActions = {
  start(): void {
    // implementation
  },
  stop(): void {
    // implementation
  },
  // reset is optional, omitted
};

// Object type with generic methods (if supported)
type Container = {
  getValue(): string;
  setValue(value: string): void;
  clear?(): void;
};

let container: Container = {
  getValue(): string {
    return "test";
  },
  setValue(value: string): void {
    // implementation
  },
};

// Nested object types with shorthand methods
type Service = {
  api: {
    get(url: string): string;
    post(url: string, data: string): string;
    delete?(url: string): void; // optional method
  };
  config: {
    timeout: number;
    retries: number;
  };
};

let service: Service = {
  api: {
    get(url: string): string {
      return "GET " + url;
    },
    post(url: string, data: string): string {
      return "POST " + url + " " + data;
    },
    // delete is optional, omitted
  },
  config: {
    timeout: 5000,
    retries: 3,
  },
};

// Test all functionality
let result = "failure";
if (
  sum === 15 &&
  diff === 5 &&
  product === 50 &&
  greeting === "Hello, John" &&
  name === "John" &&
  container.getValue() === "test" &&
  service.api.get("/users") === "GET /users" &&
  service.api.post("/users", "data") === "POST /users data"
) {
  result = "success";
}

result;

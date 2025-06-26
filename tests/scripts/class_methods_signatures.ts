// expect: Alice is 26 years old
// Test various method signatures and features

class MethodSignatures {
  name: string;

  constructor(name: string) {
    this.name = name;
  }

  // Basic method
  getName(): string {
    return this.name;
  }

  // Method with parameters and return type
  formatAge(age: number): string {
    return `${this.name} is ${age} years old`;
  }

  // Method with optional parameters
  greet(greeting?: string): string {
    return `${greeting || "Hello"}, ${this.name}!`;
  }

  // Method with default parameters
  introduce(title: string = "Mr/Ms"): string {
    return `${title} ${this.name}`;
  }

  // Method with multiple parameters
  fullInfo(age: number, city: string, active: boolean = true): string {
    return `${this.name}, ${age}, ${city}, ${active}`;
  }

  // Method returning void
  log(message: string): void {
    // Log message (in real implementation)
  }

  // Method with union type parameters
  setId(id: string | number): void {
    // Set ID
  }
}

let person = new MethodSignatures("Alice");
person.formatAge(26);

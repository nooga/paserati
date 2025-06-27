// expect: John (valid: true)
// Test property getters and setters

class Person {
  private _name: string;
  private _age: number;

  constructor(name: string, age: number) {
    this._name = name;
    this._age = age;
  }

  // Getter for name
  get name(): string {
    return this._name;
  }

  // Setter for name with validation
  set name(value: string) {
    if (value.length < 2) {
      throw new Error("Name must be at least 2 characters");
    }
    this._name = value;
  }

  // Getter for age
  get age(): number {
    return this._age;
  }

  // Setter for age with validation
  set age(value: number) {
    if (value < 0 || value > 150) {
      throw new Error("Age must be between 0 and 150");
    }
    this._age = value;
  }

  // Read-only computed property
  get isAdult(): boolean {
    return this._age >= 18;
  }

  // Another computed property
  get displayName(): string {
    return this._name.toUpperCase();
  }

  // Method using getters
  toString(): string {
    return `${this.name} (valid: ${this.isAdult})`;
  }
}

// Static getters/setters
class Config {
  private static _debug: boolean = false;

  static get debug(): boolean {
    return Config._debug;
  }

  static set debug(value: boolean) {
    Config._debug = value;
  }
}

let person = new Person("John", 25);
person.name = "John"; // Uses setter
let name = person.name; // Uses getter
person.toString(); // Final expression should return this value

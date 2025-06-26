// expect: 42
// Test class fields with type annotations and initializers

class TypedFields {
  // Basic typed fields
  name: string;
  age: number;
  isActive: boolean;

  // Fields with initializers
  score: number = 100;
  level: string = "beginner";
  enabled: boolean = true;

  // Optional fields
  nickname?: string;
  config?: object;

  // Readonly fields
  id: number = 42;
  created: string = "2024";

  constructor(name: string, age: number) {
    this.name = name;
    this.age = age;
  }
}

let user = new TypedFields("Alice", 25);
console.log(JSON.stringify(user));
user.id;

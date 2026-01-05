// Classes Demo - Inheritance, Private Fields, Static Members

class Entity {
  static count = 0;
  
  readonly id: string;
  createdAt: number;

  constructor(id: string) {
    this.id = id;
    this.createdAt = Date.now();
    Entity.count++;
  }
}

class User extends Entity {
  #password: string;  // private field
  name: string;
  
  constructor(id: string, name: string, password: string) {
    super(id);
    this.name = name;
    this.#password = password;
  }

  checkPassword(input: string): boolean {
    return this.#password === input;
  }

  greet(): string {
    return `Hello, I'm ${this.name}`;
  }
}

class Admin extends User {
  permissions: string[];

  constructor(id: string, name: string, password: string, permissions: string[]) {
    super(id, name, password);
    this.permissions = permissions;
  }

  greet(): string {
    return `${super.greet()} (Admin)`;
  }

  hasPermission(perm: string): boolean {
    return this.permissions.includes(perm);
  }
}

// Demo
console.log("=== Classes Demo ===\n");

const user = new User("u1", "Alice", "secret123");
const admin = new Admin("a1", "Bob", "admin456", ["read", "write", "delete"]);

console.log(user.greet());
console.log(admin.greet());
console.log(`Password check: ${user.checkPassword("secret123")}`);
console.log(`Admin can delete: ${admin.hasPermission("delete")}`);
console.log(`Total entities created: ${Entity.count}`);

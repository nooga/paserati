// Test file for shorthand method syntax in interfaces
// expect: true

// Interface with shorthand method syntax
interface EventHandler {
  onStart(): void;
  onStop(): void;
  onUpdate(data: string): void;
  onError?(error: string): void; // optional method
}

// Interface with mixed syntax (properties and methods)
interface UserService {
  baseUrl: string;
  timeout: number;

  getUser(id: number): string;
  createUser(name: string, email: string): string;
  deleteUser?(id: number): void; // optional method
}

let userService: UserService = {
  baseUrl: "https://api.example.com",
  timeout: 5000,

  getUser(id: number): string {
    return "User " + id;
  },

  createUser(name: string, email: string): string {
    return "Created: " + name + " (" + email + ")";
  },

  // deleteUser is optional, omitted
};

// Interface extending another interface with methods
interface BaseRepository {
  save(item: string): void;
  find(id: number): string;
}

interface ExtendedRepository extends BaseRepository {
  findAll(): string[];
  delete?(id: number): void; // optional method
}

let repo: ExtendedRepository = {
  save(item: string): void {
    // implementation
  },

  find(id: number): string {
    return "Item " + id;
  },

  findAll(): string[] {
    return ["item1", "item2"];
  },

  // delete is optional, omitted
};

// Interface with methods that return complex types
interface DataProcessor {
  process(input: string): { result: string; success: boolean };
  validate(data: string): boolean;
  transform?(input: string): string; // optional
}

let processor: DataProcessor = {
  process(input: string): { result: string; success: boolean } {
    return { result: "processed: " + input, success: true };
  },

  validate(data: string): boolean {
    return data.length > 0;
  },

  // transform is optional, omitted
};

// Interface with generic-like behavior (using any for now)
interface Container {
  getValue(): any;
  setValue(value: any): void;
  hasValue(): boolean;
  clear?(): void; // optional
}

let container: Container = {
  getValue(): any {
    return "test-value";
  },

  setValue(value: any): void {
    // implementation
  },

  hasValue(): boolean {
    return true;
  },

  // clear is optional, omitted
};

// Test all functionality
let user = userService.getUser(123);
let created = userService.createUser("John", "john@test.com");
let item = repo.find(1);
let items = repo.findAll();
let processed = processor.process("test");
let isValid = processor.validate("test");
let value = container.getValue();
let hasValue = container.hasValue();

user === "User 123" &&
  created === "Created: John (john@test.com)" &&
  item === "Item 1" &&
  items.length === 2 &&
  items[0] === "item1" &&
  items[1] === "item2" &&
  processed.result === "processed: test" &&
  processed.success === true &&
  isValid === true &&
  value === "test-value" &&
  hasValue === true;

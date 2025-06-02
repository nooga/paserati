// Test file for optional fields and methods in object types and interfaces
// expect: success

// Object type with optional fields
type UserProfile = {
  name: string;
  email: string;
  age?: number; // optional field
  phone?: string; // optional field
  address?: {
    street: string;
    city: string;
    zipCode?: string; // nested optional field
  };
};

// Test with all fields provided
let fullProfile: UserProfile = {
  name: "John Doe",
  email: "john@example.com",
  age: 30,
  phone: "123-456-7890",
  address: {
    street: "123 Main St",
    city: "Anytown",
    zipCode: "12345",
  },
};

// Test with only required fields
let minimalProfile: UserProfile = {
  name: "Jane Smith",
  email: "jane@example.com",
};

// Test with some optional fields
let partialProfile: UserProfile = {
  name: "Bob Wilson",
  email: "bob@example.com",
  age: 25,
  address: {
    street: "456 Oak Ave",
    city: "Somewhere",
    // zipCode is optional, omitted
  },
};

// Interface with optional fields and methods
interface DatabaseService {
  host: string;
  port: number;
  database: string;
  username?: string; // optional field
  password?: string; // optional field

  connect(): boolean;
  disconnect(): void;
  query(sql: string): string[];
  backup?(filename: string): boolean; // optional method
  restore?(filename: string): boolean; // optional method
}

// Implementation with all optional members
let fullService: DatabaseService = {
  host: "localhost",
  port: 5432,
  database: "mydb",
  username: "admin",
  password: "secret",

  connect(): boolean {
    return true;
  },

  disconnect(): void {
    // implementation
  },

  query(sql: string): string[] {
    return ["result1", "result2"];
  },

  backup(filename: string): boolean {
    return true;
  },

  restore(filename: string): boolean {
    return true;
  },
};

// Implementation with only required members
let minimalService: DatabaseService = {
  host: "localhost",
  port: 5432,
  database: "testdb",

  connect(): boolean {
    return true;
  },

  disconnect(): void {
    // implementation
  },

  query(sql: string): string[] {
    return ["test"];
  },

  // username, password, backup, and restore are optional, omitted
};

// Implementation with some optional members
let partialService: DatabaseService = {
  host: "remote-host",
  port: 3306,
  database: "proddb",
  username: "user",
  // password is optional, omitted

  connect(): boolean {
    return true;
  },

  disconnect(): void {
    // implementation
  },

  query(sql: string): string[] {
    return ["data"];
  },

  backup(filename: string): boolean {
    return true;
  },

  // restore is optional, omitted
};

// Object type with optional methods only
type EventEmitter = {
  addEventListener(event: string, handler: () => void): void;
  removeEventListener(event: string, handler: () => void): void;
  emit(event: string): void;

  // Optional utility methods
  once?(event: string, handler: () => void): void;
  off?(event: string, handler: () => void): void;
  listenerCount?(event: string): number;
};

let emitter: EventEmitter = {
  addEventListener(event: string, handler: () => void): void {
    // implementation
  },

  removeEventListener(event: string, handler: () => void): void {
    // implementation
  },

  emit(event: string): void {
    // implementation
  },

  once(event: string, handler: () => void): void {
    // optional method implementation
  },

  // off and listenerCount are optional, omitted
};

// Interface extending another with optional members
interface BaseConfig {
  name: string;
  version: string;
}

interface ExtendedConfig extends BaseConfig {
  description?: string; // optional field
  author?: string; // optional field

  validate(): boolean;
  save?(): void; // optional method
}

let config: ExtendedConfig = {
  name: "MyApp",
  version: "1.0.0",
  description: "A test application",
  // author is optional, omitted

  validate(): boolean {
    return true;
  },

  save(): void {
    // optional method implementation
  },
};

// Test accessing optional fields and methods
let hasAge = fullProfile.age !== undefined;
let hasPhone = partialProfile.phone !== undefined;
let hasUsername = fullService.username !== undefined;
let hasBackup = typeof fullService.backup === "function";
let hasDescription = config.description !== undefined;

// Test functionality
let connected = fullService.connect();
let results = minimalService.query("SELECT * FROM test");
let isValid = config.validate();

let result = "failure";
if (
  fullProfile.name === "John Doe" &&
  fullProfile.age === 30 &&
  minimalProfile.name === "Jane Smith" &&
  minimalProfile.age === undefined &&
  partialProfile.address !== undefined &&
  partialProfile.address.zipCode === undefined &&
  connected === true &&
  results.length === 1 &&
  results[0] === "test" &&
  isValid === true &&
  hasAge === true &&
  hasPhone === false &&
  hasUsername === true &&
  hasBackup === true &&
  hasDescription === true
) {
  result = "success";
}

result;

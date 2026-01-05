// Runtime Type Reflection - Paserati's Unique Feature
// Generate JSON Schema from TypeScript types at runtime

// === Complex nested types ===

interface Address {
  street: string;
  city: string;
  country: string;
  postalCode?: string;
}

interface ContactInfo {
  email: string;
  phone?: string;
  address: Address;
}

interface User {
  id: string;
  name: string;
  role: "admin" | "editor" | "viewer";
  contact: ContactInfo;
  permissions: string[];
  metadata?: {
    createdAt: string;
    lastLogin?: string;
    tags: Array<string>;
  };
}

// Generate schema at runtime - impossible in standard TypeScript!
const userSchema = Paserati.reflect<User>().toJSONSchema();
console.log("=== User Schema (nested objects) ===");
console.log(JSON.stringify(userSchema, null, 2));

// === Generic types ===

interface ApiResponse<T> {
  success: boolean;
  data: T;
  error?: {
    code: number;
    message: string;
  };
}

interface PaginatedList<T> {
  items: Array<T>;
  total: number;
  page: number;
  pageSize: number;
}

// Reflect generic instantiations
const apiResponseSchema = Paserati.reflect<ApiResponse<User>>().toJSONSchema();
console.log("\n=== ApiResponse<User> Schema ===");
console.log(JSON.stringify(apiResponseSchema, null, 2));

const paginatedUsersSchema = Paserati.reflect<PaginatedList<User>>().toJSONSchema();
console.log("\n=== PaginatedList<User> Schema ===");
console.log(JSON.stringify(paginatedUsersSchema, null, 2));

// === Practical use case: API request validation ===

interface CreateUserRequest {
  username: string;
  email: string;
  password: string;
  profile?: {
    displayName?: string;
    bio?: string;
    avatar?: string;
  };
}

// Generate schema for request validation
const requestSchema = Paserati.reflect<CreateUserRequest>().toJSONSchema();
console.log("\n=== CreateUserRequest Schema (for validation) ===");
console.log(JSON.stringify(requestSchema, null, 2));

// Use the schema for validation (in real code, you'd use a JSON Schema library)
function validate(data: unknown, schema: object): boolean {
  // Simplified - real impl would validate against schema
  return typeof data === "object" && data !== null;
}

const testData = { username: "alice", email: "alice@example.com", password: "secret" };
console.log("\nValidation result:", validate(testData, requestSchema));

// Runtime Type Reflection - Paserati's Unique Feature
// Generate JSON Schema from TypeScript types at runtime

// Define your types as usual
interface User {
  id: string;
  name: string;
  email: string;
  age?: number;
  role: "admin" | "user" | "guest";
}

interface APIRequest {
  endpoint: string;
  method: "GET" | "POST" | "PUT" | "DELETE";
  body?: {
    [key: string]: string | number | boolean;
  };
  headers?: string[];
}

// Generate JSON Schema at runtime - impossible in standard TypeScript!
const userSchema = Paserati.reflect<User>().toJSONSchema();
const requestSchema = Paserati.reflect<APIRequest>().toJSONSchema();

console.log("User Schema:");
console.log(JSON.stringify(userSchema, null, 2));

console.log("\nAPI Request Schema:");
console.log(JSON.stringify(requestSchema, null, 2));

// Practical use case: LLM tool definitions
interface SearchParams {
  query: string;
  maxResults?: number;
  filters?: {
    dateRange?: "day" | "week" | "month";
    category?: string;
  };
}

const tools = [
  {
    name: "search",
    description: "Search for information",
    parameters: Paserati.reflect<SearchParams>().toJSONSchema()
  }
];

console.log("\nLLM Tool Definition:");
console.log(JSON.stringify(tools, null, 2));

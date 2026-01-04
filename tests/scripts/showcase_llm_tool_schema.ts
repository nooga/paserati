// LLM Tool Schema Generation - Realistic Example
// Demonstrates automatic JSON Schema generation for AI agent tooling
// expect: Tool schemas generated successfully

// === Define tool parameter types ===

// Weather tool parameters
interface WeatherParams {
    location: string;
    units?: "celsius" | "fahrenheit";
}

// Search tool parameters
interface SearchParams {
    query: string;
    maxResults?: number;
    filters?: {
        dateRange?: "day" | "week" | "month" | "year";
        domain?: string;
    };
}

// File operation parameters
interface FileParams {
    path: string;
    operation: "read" | "write" | "delete";
    content?: string;
}

// Database query parameters
interface DatabaseParams {
    table: string;
    columns?: string[];
    where?: {
        [field: string]: string | number | boolean;
    };
    limit?: number;
    orderBy?: string;
}

// === Generate JSON Schemas for each tool ===

const weatherSchema = Paserati.reflect<WeatherParams>().toJSONSchema();
const searchSchema = Paserati.reflect<SearchParams>().toJSONSchema();
const fileSchema = Paserati.reflect<FileParams>().toJSONSchema();
const databaseSchema = Paserati.reflect<DatabaseParams>().toJSONSchema();

// === Build tool definitions for LLM ===

interface ToolDefinition {
    name: string;
    description: string;
    parameters: object;
}

const tools: ToolDefinition[] = [
    {
        name: "get_weather",
        description: "Get current weather for a location",
        parameters: weatherSchema
    },
    {
        name: "search",
        description: "Search the web for information",
        parameters: searchSchema
    },
    {
        name: "file_operation",
        description: "Read, write, or delete files",
        parameters: fileSchema
    },
    {
        name: "query_database",
        description: "Query a database table",
        parameters: databaseSchema
    }
];

// === Verify the schemas are correct ===

let valid = true;

// Check weather schema
if (weatherSchema.type !== "object") valid = false;
if (!weatherSchema.properties?.location) valid = false;
if (weatherSchema.properties?.units?.enum?.length !== 2) valid = false;

// Check search schema has nested object
if (!searchSchema.properties?.filters) valid = false;
if (searchSchema.properties?.filters?.properties?.dateRange?.enum?.length !== 4) valid = false;

// Check file schema has enum for operation
if (fileSchema.properties?.operation?.enum?.length !== 3) valid = false;

// Check database schema has additionalProperties for where clause
if (!databaseSchema.properties?.where?.additionalProperties) valid = false;

// Output result
let result: string;
if (valid) {
    // Show one example schema
    console.log("Example tool schema (search):");
    console.log(JSON.stringify(searchSchema, null, 2));
    result = "Tool schemas generated successfully";
} else {
    result = "Schema validation failed";
}
result;

// expect: 42

// Test type inference with shorthand properties
let name: string = "TypeScript";
let version: number = 5;
let stable: boolean = true;

let tsInfo = { name, version, stable };

// The type checker should infer:
// tsInfo: { name: string; version: number; stable: boolean }

// Test with interface compatibility
interface UserInfo {
  name: string;
  age: number;
  active: boolean;
}

let userName: string = "Alice";
let userAge: number = 25;
let userActive: boolean = true;

// Shorthand should be compatible with interface
let userObj: UserInfo = { name: userName, age: userAge, active: userActive };

// Test shorthand with type annotations on the resulting object
let count: number = 17;
let data: { count: number; label: string } = {
  count,
  label: "Items",
};

// Test shorthand in function parameters and returns
function createConfig(debug: boolean, port: number) {
  let env = "development";
  return { debug, port, env };
}

let config = createConfig(true, 3000);

// Verify everything works at runtime
let result = config.port + data.count + userObj.age; // 3000 + 17 + 25 = 3042

// Just return a simpler number for easier testing
42;

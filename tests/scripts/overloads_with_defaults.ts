// Test function overloads integration with optional and default parameters
// expect: success

console.log("Testing overload integration with defaults and optionals");

// Test 1: Simple overloads (existing functionality)
function basic(x: string): string;
function basic(x: number): number;
function basic(x: string | number): string | number {
  return x;
}

console.log("=== Test 1: Basic overloads work ===");
console.log(basic("hello"));
console.log(basic(42));

// Test 2: Functions with default parameters (not overloaded)
function withDefaults(name: string, greeting: string = "Hello"): string {
  return greeting + " " + name + "!";
}

console.log("=== Test 2: Default parameters work ===");
console.log(withDefaults("John"));
console.log(withDefaults("Jane", "Hi"));

// Test 3: Functions with optional parameters (not overloaded)
function withOptionals(first: string, last?: string): string {
  if (last) {
    return first + " " + last;
  }
  return first;
}

console.log("=== Test 3: Optional parameters work ===");
console.log(withOptionals("John"));
console.log(withOptionals("John", "Doe"));

// Test 4: Object methods with defaults and optionals (shorthand methods)
const obj = {
  greetDefault(name: string = "World"): string {
    return "Hello " + name + "!";
  },

  greetOptional(name: string, suffix?: string): string {
    return "Hello " + name + (suffix || "!");
  },
};

console.log("=== Test 4: Shorthand methods with defaults/optionals ===");
console.log(obj.greetDefault());
console.log(obj.greetDefault("Alice"));
console.log(obj.greetOptional("Bob"));
console.log(obj.greetOptional("Bob", "?"));

console.log("Integration test completed - all features work independently");
("success");

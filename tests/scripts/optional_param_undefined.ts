// expect: hi world
// Test: optional parameters accept explicit undefined argument
// param?: string should accept undefined since it's equivalent to param?: string | undefined

function greet(name?: string, greeting?: string): string {
  return (greeting || "hello") + " " + (name || "world");
}

// All of these should be valid
greet();
greet("Alice");
greet(undefined);
greet(undefined, undefined);
greet("Bob", undefined);
greet(undefined, "hi");

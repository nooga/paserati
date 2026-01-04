// Generic type inference from object literal arguments
// expect: hello-42

// Simple generic function with object parameter
function getValue<T>(p: { value: T }): T {
  return p.value;
}

// T should be inferred as string from the object literal
const str = getValue({ value: "hello" });

// T should be inferred as number from the object literal
const num = getValue({ value: 42 });

// Combine results
str + "-" + num;

// Test function parameter object destructuring with defaults
function createGreeting({greeting = "Hello", name = "World"}) {
  return greeting + " " + name + "!";
}

let result1 = createGreeting({greeting: "Hi", name: "there"});
let result2 = createGreeting({greeting: "Goodbye"});
let result3 = createGreeting({name: "Alice"});
let result4 = createGreeting({});

`${result1}, ${result2}, ${result3}, ${result4}`;
// expect: Hi there!, Goodbye World!, Hello Alice!, Hello World!
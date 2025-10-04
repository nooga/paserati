// expect: done
// Test for-of loops with object destructuring

// Basic object destructuring in for-of
const people = [
  { name: "Alice", age: 30 },
  { name: "Bob", age: 25 }
];
for (const { name, age } of people) {
  console.log(name + ":" + age);
}

// Object destructuring with renaming
const items = [
  { id: 1, label: "item1" },
  { id: 2, label: "item2" }
];
for (const { id: itemId, label: itemLabel } of items) {
  console.log(itemId + "-" + itemLabel);
}

// Object destructuring with default values
const configs = [
  { port: 8080 },
  { host: "localhost" }
];
for (const { port = 3000, host = "0.0.0.0" } of configs) {
  console.log(host + ":" + port);
}

// Simple object access (nested destructuring not yet supported)
const users = [
  { name: "User1", city: "NY" },
  { name: "User2", city: "LA" }
];
for (const { name, city } of users) {
  console.log(name + " in " + city);
}

("done");

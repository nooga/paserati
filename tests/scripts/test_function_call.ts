function greet(name: string): string {
  return "Hello, " + name + "!";
}

let obj = { prefix: "Mr. " };

function addPrefix(this: any, name: string): string {
  return this.prefix + name;
}

// Test Function.prototype.call
console.log(greet.call(undefined, "World"));
console.log(addPrefix.call(obj, "Smith"));
function add(a: number, b: number): number {
  return a + b;
}

console.log("Testing minimal spread");
add(...[1, 2]);
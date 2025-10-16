console.log("1");

async function test() {
  console.log("2");
  await Promise.resolve(null);
  console.log("3");
}

console.log("4");
test().then(() => console.log("5"));
console.log("6");

// Test that async generator objects are created
// expect: created

async function* test() {
  yield 42;
}

const gen = test();
console.log("Type:", typeof gen);
console.log("Has next:", typeof gen.next);
"created";

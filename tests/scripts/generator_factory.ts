// Generator factory function test
// expect: hello world

function createGenerator(prefix: string) {
  function* innerGenerator() {
    yield prefix + " world";
  }
  return innerGenerator();
}

const gen = createGenerator("hello");
console.log("Generator from factory:", typeof gen);
const result = gen.next();
console.log("Result:", result.value);

result.value;

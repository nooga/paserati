// expect: Function: 1,2,3 | Method: 4,5,6
// Test generator function vs generator method - compare type system treatment

// Regular generator function
function* generatorFunction() {
  yield 1;
  yield 2;
  yield 3;
}

// Class with generator method
class TestClass {
  *generatorMethod() {
    yield 4;
    yield 5;
    yield 6;
  }
}

const instance = new TestClass();

// Test for...of with generator function
let functionResult = "Function: ";
for (let value of generatorFunction()) {
  functionResult += value + ",";
}
functionResult = functionResult.slice(0, -1);

// Test for...of with generator method
let methodResult = "Method: ";
for (let value of instance.generatorMethod()) {
  methodResult += value + ",";
}
methodResult = methodResult.slice(0, -1);

`${functionResult} | ${methodResult}`;
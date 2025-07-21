// expect: Function: 1,2,3 | Object: 4,5,6
// Test generator function vs object generator method

// Regular generator function
function* generatorFunction() {
  yield 1;
  yield 2;
  yield 3;
}

// Object with generator method
const obj = {
  *generatorMethod() {
    yield 4;
    yield 5;
    yield 6;
  }
};

// Test for...of with generator function
let functionResult = "Function: ";
for (let value of generatorFunction()) {
  functionResult += value + ",";
}
functionResult = functionResult.slice(0, -1);

// Test for...of with object generator method
let objectResult = "Object: ";
for (let value of obj.generatorMethod()) {
  objectResult += value + ",";
}
objectResult = objectResult.slice(0, -1);

`${functionResult} | ${objectResult}`;
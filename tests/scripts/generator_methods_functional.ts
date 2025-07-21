// expect: Numbers: 1,2,3 | Words: hello,world | Iterator: 10,20,30
// Test functional generator methods in object literals with yield and for...of

const generators = {
  *numbers() {
    yield 1;
    yield 2;
    yield 3;
  },
  
  *words() {
    yield "hello";
    yield "world";
  },
  
  *[Symbol.iterator]() {
    yield 10;
    yield 20;
    yield 30;
  }
};

// Test regular generator method
let numbersResult = "Numbers: ";
for (let num of generators.numbers()) {
  numbersResult += num + ",";
}
numbersResult = numbersResult.slice(0, -1);

// Test generator method with strings
let wordsResult = "Words: ";
for (let word of generators.words()) {
  wordsResult += word + ",";
}
wordsResult = wordsResult.slice(0, -1);

// Test Symbol.iterator generator method  
let iteratorResult = "Iterator: ";
for (let val of generators[Symbol.iterator]()) {
  iteratorResult += val + ",";
}
iteratorResult = iteratorResult.slice(0, -1);

`${numbersResult} | ${wordsResult} | ${iteratorResult}`;
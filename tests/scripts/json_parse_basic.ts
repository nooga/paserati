// Test JSON.parse with basic types
let num = JSON.parse("42");
let str = JSON.parse('"hello"');
let bool = JSON.parse("true");
let nullVal = JSON.parse("null");

let basicCorrect =
  num === 42 && str === "hello" && bool === true && nullVal === null;

// expect: true
basicCorrect;

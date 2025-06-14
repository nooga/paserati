// Test to see if function has its own chunk
function testFunc() {
  console.log("Inside testFunc");
  return 42;
}

console.log("Script level code");
console.log("Calling testFunc:", testFunc());
// expect: true  
// Test bc -> bc -> native exception pattern
let caught = false;
function userFunction() {
  return JSON.parse("{invalid json}");
}

function intermediateFunction() {
  return userFunction();
}

try {
  intermediateFunction();
} catch(e) {
  console.log("Caught bc->bc->native error:", e.message);
  caught = true;
}
caught;
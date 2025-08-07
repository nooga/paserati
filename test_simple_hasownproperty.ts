// Simple test similar to the failing test262 case
function f1(){
  return arguments.hasOwnProperty("callee");
}

try {
  let result = f1();
  console.log("Result:", result);
  console.log("Type:", typeof result);
} catch(e) {
  console.log("Error:", e);
}
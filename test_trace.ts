// Simplified version of the failing test
const iter = {
  [Symbol.iterator]: function() {
    throw new Error("iterator error");
  }
};

let caught = false;
const f = async function*([x]) {
  console.log("inside generator body");
};

try {
  console.log("calling f(iter)");
  f(iter);
  console.log("f returned");
} catch (e) {
  console.log("caught error:", e.message);
  caught = true;
}

console.log("caught:", caught);

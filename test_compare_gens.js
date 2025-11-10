// Compare sync vs async generator exception handling

function testSync() {
  var iter = {};
  iter[Symbol.iterator] = function() {
    console.log("Sync: Symbol.iterator called, throwing...");
    throw new Error("Sync error");
  };

  var obj = {
    *method([x]) {}
  };

  try {
    console.log("Sync: About to call obj.method(iter)");
    obj.method(iter);
    console.log("Sync: FAIL - should have thrown");
  } catch (e) {
    console.log("Sync: SUCCESS - caught:", e.message);
  }
}

function testAsync() {
  var iter = {};
  iter[Symbol.iterator] = function() {
    console.log("Async: Symbol.iterator called, throwing...");
    throw new Error("Async error");
  };

  var obj = {
    async *method([x]) {}
  };

  try {
    console.log("Async: About to call obj.method(iter)");
    obj.method(iter);
    console.log("Async: FAIL - should have thrown");
  } catch (e) {
    console.log("Async: SUCCESS - caught:", e.message);
  }
}

console.log("=== Testing Sync Generator ===");
testSync();

console.log("\n=== Testing Async Generator ===");
testAsync();

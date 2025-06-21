// Test control flow with returns in try/catch/finally blocks

function testTryReturn(): string {
  let result = "";

  try {
    for (let i = 0; i < 3; i++) {
      if (i === 2) {
        return result + "try-early-return";
      }
      result += "try-" + i + " ";
    }
    result += "try-end ";
  } finally {
    result += "finally ";
  }

  return result + "normal-end";
}

function testCatchReturn(): string {
  let result = "";

  try {
    result += "try ";
    throw new Error("test");
  } catch (e) {
    for (let i = 0; i < 2; i++) {
      if (i === 1) {
        return result + "catch-early-return";
      }
      result += "catch-" + i + " ";
    }
    result += "catch-end ";
  } finally {
    result += "finally ";
  }

  return result + "normal-end";
}

function testFinallyReturn(): string {
  let result = "";

  try {
    result += "try ";
    return result + "try-return";
  } finally {
    result += "finally ";
    if (result.length > 10) {
      return result + "finally-return";
    }
  }

  return result + "normal-end";
}

const r =
  testTryReturn() + " | " + testCatchReturn() + " | " + testFinallyReturn();
console.log(r);
// expect: try-0 try-1 try-early-return | try catch-0 catch-early-return | try finally finally-return
r;

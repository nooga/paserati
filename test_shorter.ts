let x = [10, 20, 30];
let y = 5;

function testUpvalues() {
  let z = [100, 200, 300];
  let w = 50;

  function inner() {
    z[0] += 1; // Simple operations only
    z[1] -= 10;
    w += 5;
  }

  inner();
  return [z, w];
}

// Do fewer operations on y
y += 3;
y -= 1;
y *= 2;

// Do fewer operations on x
x[0] += 1;
x[1] -= 5;

let upvalueResult = testUpvalues();
console.log("upvalueResult:", upvalueResult);

let finalResult = [x, y, upvalueResult];
console.log("finalResult:", finalResult);

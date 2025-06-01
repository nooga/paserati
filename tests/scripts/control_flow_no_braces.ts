// Test for control flow statements without braces
// Tests if, else, while, for, do-while with and without braces in various combinations
// expect: 20

function testControlFlowNoBraces(): number {
  let result = 0;
  let counter = 0;

  // Test 1: Simple if without braces
  if (true) result = 1;

  // Test 2: if/else without braces
  if (false) result = 100; // Should not execute
  else result = 2;

  // Test 3: Nested if/else without braces
  if (false) result = 100;
  else if (true) result = 3;
  else result = 100;

  // Test 4: Mixed braces and no braces
  if (false) {
    result = 100;
  } else result = 4;

  // Test 5: while without braces
  counter = 0;
  while (counter < 2) counter = counter + 1;
  result = result + counter; // result = 4 + 2 = 6

  // Test 6: for without braces
  for (let i = 0; i < 3; i++) result = result + 1; // result = 6 + 3 = 9

  // Test 7: do-while without braces
  counter = 0;
  do counter = counter + 1;
  while (counter < 2);
  result = result + counter; // result = 9 + 2 = 11

  // Test 8: Nested control flow without braces
  if (true) for (let j = 0; j < 2; j++) if (j > 0) result = result + j; // result = 11 + 1 = 12

  // Test 9: Deeply nested without braces
  if (true) if (true) while (result < 15) result = result + 1; // result = 12 + 3 = 15

  // Test 10: Mixed nested statements
  for (let k = 0; k < 2; k++) {
    if (k == 0) result = result + 1; // result = 15 + 1 = 16
    else {
      while (result < 18) result = result + 1; // result = 16 + 2 = 18
    }
  }

  // Test 11: Complex nesting with all types
  if (result == 18) {
    for (let m = 0; m < 2; m++)
      if (m == 1)
        do
          result = result + 1; // result = 18 + 1 = 19
        while (false);
  } else result = 100;

  // Test 12: Single line chaining
  if (true) if (true) if (true) result = result + 1; // result = 19 + 1 = 20

  return result;
}

// Test that the function works correctly
function runTests(): number {
  let testResult = testControlFlowNoBraces();

  // Verify the expected result
  if (testResult == 20) return testResult;
  else return -1; // Error indicator
}

// Execute the test
runTests();

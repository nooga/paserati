// Type narrowing with else branches test
// expect: ok

// Test 1: Basic else narrowing with unknown
let unknown1: unknown = "hello world";
if (typeof unknown1 === "string") {
  console.log("Then branch - String length:", unknown1.length);
} else {
  // In the else branch, unknown1 is still unknown (but we know it's not string)
  // This should still fail since unknown1 is still unknown
  // console.log("Else branch - This should fail:", unknown1.length);
  console.log("Else branch - value is not a string");
}

// Test 2: Testing with actual number
let unknown2: unknown = 42;
if (typeof unknown2 === "string") {
  console.log("Won't reach here - number is not string");
} else {
  console.log("Else branch - correctly identified as not string");
}

// Test 3: Sequential narrowing
let unknown3: unknown = true;
if (typeof unknown3 === "string") {
  console.log("Not a string");
} else if (typeof unknown3 === "number") {
  console.log("Not a number either");
} else if (typeof unknown3 === "boolean") {
  console.log("Finally found boolean! Value:", unknown3);
} else {
  console.log("Something else entirely");
}

// Test 4: Nested if/else
let unknown4: unknown = "test";
if (typeof unknown4 === "number") {
  console.log("Is number:", unknown4 + 10);
} else {
  // In else, we know it's not a number, but it's still unknown
  if (typeof unknown4 === "string") {
    console.log("Nested check - is string:", unknown4.length);
  } else {
    console.log("Not number and not string");
  }
}

console.log("Type narrowing else tests completed");
("ok");

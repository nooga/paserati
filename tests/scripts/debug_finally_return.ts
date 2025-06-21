// Simple inline test
let x = "";
try {
  x += "try";
  console.log("try");
} finally {
  x += "finally";
  console.log("finally");
}

x;
// expect: tryfinally

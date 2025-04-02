// expect: 42

/* This is a simple multiline comment */
let a = 10;

/*
  This comment
  spans multiple
  lines.
*/
let b = 20;

let c = /* This comment is inline */ 12;

let result = a + b + c;

result;
/* Another comment at the end */

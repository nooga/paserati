// A continue that leaves a with body through a finally drops the with object
// once the finally has run: the next iteration must not see it
// no-typecheck
// expect: out,leak,out,leak,out,out,out
var o = { x: "leak" };
var x = "out";
var seen = [];
for (var i = 0; i < 2; i++) {
  with ({}) seen.push(x);
  with (o) { try { continue; } finally { seen.push(x); } }
}
for (var k of [1, 2]) { with ({}) seen.push(x); with (o) { try { continue; } finally {} } }
with ({}) seen.push(x);
seen.join(",");

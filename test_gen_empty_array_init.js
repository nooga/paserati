// Minimal reproduction of ary-ptrn-elem-ary-empty-init regression

var initCount = 0;
var iterCount = 0;
var iter = function*() { iterCount += 1; }();

console.log("iter:", iter);

var callCount = 0;
var f;
f = function*([[] = function() { initCount += 1; return iter; }()]) {
  console.log("Inside generator body");
  console.log("initCount:", initCount);
  console.log("iterCount:", iterCount);
  callCount = callCount + 1;
};

console.log("About to call f([])");
var gen = f([]);
console.log("About to call gen.next()");
gen.next();
console.log("callCount:", callCount);

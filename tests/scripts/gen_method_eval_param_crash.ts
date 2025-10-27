// no-typecheck

// expect: with eval OK
// expect: probe1: inside
// expect: probe2: inside

var x = "outside";
var probe1, probe2;
({
  *m(
    _ = (eval('var x = "inside";'),
    (probe1 = function () {
      return x;
    })),
    __ = (probe2 = function () {
      return x;
    })
  ) {},
})
  .m()
  .next();
console.log("with eval OK");
console.log("probe1:", probe1());
console.log("probe2:", probe2());
"with eval OK";

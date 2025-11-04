var x = 0;
var assert = {
  sameValue: function(a, b) {
    console.log("sameValue:", a, "===", b);
  }
};
function* g() {
  yield 1;
  yield 2;
}
var obj = {
  *method([,]) {
    console.log("line 1");
    assert.sameValue(1, 1);
    console.log("line 2");
    x = 1;
    console.log("line 3");
  }
};
obj.method(g()).next();
console.log("x =", x);

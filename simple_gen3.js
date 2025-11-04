var x = 0;
var assert = {
  sameValue: function(a, b) {
    console.log("sameValue:", a, "===", b);
  }
};
var obj = {
  *method() {
    console.log("line 1");
    assert.sameValue(1, 1);
    console.log("line 2");
    x = 1;
    console.log("line 3");
  }
};
obj.method().next();
console.log("x =", x);

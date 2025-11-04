var x = 0;
var obj = {
  *method() {
    console.log("line 1");
    x = 1;
    console.log("line 2");
    x = 2;
    console.log("line 3");
  }
};
obj.method().next();
console.log("x =", x);

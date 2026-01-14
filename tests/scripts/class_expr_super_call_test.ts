// expect: done
// Test: super() call works in class expression (fields-run-once-on-double-super related)

class Base {
  value = 10;
}

var C = class extends Base {
  field = 5;
  constructor() {
    super();
  }
};

const instance = new C();
instance.value === 10 && instance.field === 5 ? "done" : "fail"

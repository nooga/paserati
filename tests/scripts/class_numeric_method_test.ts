// Test numeric property names in classes
// expect: function function hello getter

class C {
  42() {
    return "hello";
  }

  get 100() {
    return "getter";
  }
}

const c = new C();
`${typeof c[42]}, ${typeof c["42"]}, ${c[42]()}, ${c[100]}`;

interface Test {
  prop: string;
}

class MyClass implements Test {
  prop = "hello";
}

console.log(new MyClass().prop);
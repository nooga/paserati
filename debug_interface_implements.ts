interface Test {
    prop: string;
}

class MyClass implements Test {
    prop: string = "hello";
}

console.log("implements works");
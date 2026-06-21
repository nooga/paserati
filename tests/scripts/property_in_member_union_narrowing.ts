// expect: 1
class A {
    a!: string;
}

class B {
    b!: number;
}

class Box {
    value!: A | B;
}

let box = new Box();
box.value = new A();

if ("a" in box.value) {
    box.value.a = "ok";
} else {
    box.value.b = 1;
}

let item: A | B = new A();
if (item instanceof A) {
    item.a = "ok";
}

1;

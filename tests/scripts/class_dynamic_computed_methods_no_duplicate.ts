// expect: 7
let keyA: any = "a";
let keyB: any = "b";

class C {
    [keyA]() {
        return 3;
    }

    [keyB]() {
        return 4;
    }
}

let c = new C();
c.a() + c.b();

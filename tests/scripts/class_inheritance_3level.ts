// expect: hello|extra
class Base {
    greet() { return "hello"; }
}
class Middle extends Base {
    constructor() {
        super();
    }
    extra() { return "extra"; }
}
class Child extends Middle {}
const c = new Child();
c.greet() + "|" + c.extra();

// expect: 1
class Base {
    protected maybe?(): void;
}

class Derived extends Base {
    body() {
        super.maybe && super.maybe();
    }
}

1;

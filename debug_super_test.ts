// Test super() call
class Parent {
    constructor() {
        this.x = 42;
    }
}

class Child extends Parent {
    constructor() {
        super();
        this.y = 99;
    }
}

const c = new Child();
console.log('c:', c);
console.log('x:', c.x);
console.log('y:', c.y);

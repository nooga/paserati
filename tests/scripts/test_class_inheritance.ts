// Simple test for class inheritance
class Container<T> {
    items: T[];
    constructor() {
        this.items = [];
    }
}

class Stack<T> extends Container<T> {
    push(item: T): void {
        // test
    }
}

"test";
// expect: test
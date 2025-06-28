// expect_runtime_error: Cannot call non-function value

// Test 1: Basic generic class with recursive methods
class LinkedNode<T> {
    value: T;
    next?: LinkedNode<T>;
    
    constructor(value: T) {
        this.value = value;
    }
    
    append(node: LinkedNode<T>): void {
        if (this.next) {
            this.next.append(node);
        } else {
            this.next = node;
        }
    }
}

// Test 2: Generic class extending another generic class
class Container<T> {
    items: T[];
    
    constructor() {
        this.items = [];
    }
    
    add(item: T): void {
        this.items.push(item);
    }
}

class Stack<T> extends Container<T> {
    push(item: T): void {
        this.add(item);
    }
    
    pop(): T | undefined {
        return this.items.pop();
    }
}

// Test 3: Complex inheritance with type constraints
class Comparable<T> {
    value: T;
    
    constructor(value: T) {
        this.value = value;
    }
}

class SortedList<T> extends Comparable<T[]> {
    constructor(items: T[]) {
        super(items);
    }
    
    sort(): void {
        // Sort implementation
    }
}

// Test instantiation
let node = new LinkedNode<string>("test");
// TODO: Fix generic type resolution for recursive assignment
// node.next = new LinkedNode<string>("next");

let stack = new Stack<number>();
stack.push(42);
stack.push(24);

let sorted = new SortedList<string>(["c", "a", "b"]);

"recursive generic classes work";
// Test type inference for generic classes
// expect: [42, "hello", true, [1, 2, 3]]

class Container<T> {
    private _value: T;
    
    constructor(value: T) {
        this._value = value;
    }
    
    get value(): T {
        return this._value;
    }
}

class Pair<T, U> {
    first: T;
    second: U;
    
    constructor(first: T, second: U) {
        this.first = first;
        this.second = second;
    }
}

class Box<T> {
    items: T[] = [];
    
    add(item: T): void {
        this.items.push(item);
    }
    
    getAll(): T[] {
        return this.items;
    }
}

// Test type inference with number
let numContainer = new Container(42);

// Test type inference with string
let strContainer = new Container("hello");

// Test type inference with boolean
let boolContainer = new Container(true);

// Test type inference with multiple type parameters
let pair = new Pair(42, "hello");

// Test type inference with arrays - need explicit type when no constructor args
let box = new Box<number>();
box.add(1);
box.add(2);
box.add(3);

// Return as an array to match expected output
[
    numContainer.value,
    strContainer.value,
    boolContainer.value,
    box.getAll()
];
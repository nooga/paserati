// expect: 42
// Test generic classes

class Container<T> {
    private _value: T;
    
    constructor(value: T) {
        this._value = value;
    }
    
    get value(): T {
        return this._value;
    }
    
    set value(newValue: T) {
        this._value = newValue;
    }
    
    toString(): string {
        return `Container(${this._value})`;
    }
}

class Pair<T, U> {
    first: T;
    second: U;
    
    constructor(first: T, second: U) {
        this.first = first;
        this.second = second;
    }
    
    getFirst(): T {
        return this.first;
    }
    
    getSecond(): U {
        return this.second;
    }
    
    swap(): Pair<U, T> {
        return new Pair(this.second, this.first);
    }
}

class Stack<T> {
    private items: T[] = [];
    
    push(item: T): void {
        this.items.push(item);
    }
    
    pop(): T | undefined {
        return this.items.pop();
    }
    
    peek(): T | undefined {
        return this.items[this.items.length - 1];
    }
    
    get size(): number {
        return this.items.length;
    }
    
    isEmpty(): boolean {
        return this.items.length === 0;
    }
}

// Generic class with constraints
class NumberContainer<T extends number> {
    value: T;
    
    constructor(value: T) {
        this.value = value;
    }
    
    add(other: T): number {
        return this.value + other;
    }
}

let numContainer = new Container<number>(42);
numContainer.value;
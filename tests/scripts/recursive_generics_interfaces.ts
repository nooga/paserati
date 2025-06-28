// expect: recursive generic interfaces work

// Test 1: Basic recursive interface
interface Node<T> {
    value: T;
    next?: Node<T>;
}

// Test 2: Generic interface extending another generic interface
interface List<T> {
    head?: Node<T>;
    size: number;
}

interface Stack<T> extends List<T> {
    push(item: T): void;
    pop(): T | undefined;
}

// Test 3: Mutually recursive interfaces
interface TreeNode<T> {
    data: T;
    children: Forest<T>;
}

interface Forest<T> {
    nodes: TreeNode<T>[];
    isEmpty(): boolean;
}

// Test 4: Complex inheritance chain
interface Container<T> {
    items: T[];
}

interface SortedContainer<T> extends Container<T> {
    sort(): void;
}

interface SearchableContainer<T> extends SortedContainer<T> {
    find(item: T): number;
}

// Test assignments
let node: Node<string> = {
    value: "first",
    next: {
        value: "second",
        next: {
            value: "third"
        }
    }
};

let stack: Stack<number> = {
    head: { value: 42 },
    size: 1,
    push: function(item: number) {},
    pop: function() { return undefined; }
};

"recursive generic interfaces work";
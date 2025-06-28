// expect: recursive generic type aliases work

// Test 1: Basic recursive type alias
type Cons<T> = {
    val: T;
    rest?: Cons<T>;
};

// Test 2: Mutual recursion
type TreeNode<T> = {
    value: T;
    children: TreeList<T>;
};

type TreeList<T> = TreeNode<T>[];

// Test 3: Complex recursive structure
type JSON = string | number | boolean | null | JSON[] | { [key: string]: JSON };

// Test 4: Linked list with operations
type LinkedList<T> = {
    head: T;
    tail?: LinkedList<T>;
    isEmpty: false;
} | {
    isEmpty: true;
};

// Test assignments
let list: Cons<number> = {
    val: 1,
    rest: {
        val: 2,
        rest: {
            val: 3
        }
    }
};

let tree: TreeNode<string> = {
    value: "root",
    children: [
        { value: "child1", children: [] },
        { value: "child2", children: [] }
    ]
};

let jsonValue: JSON = {
    name: "test",
    values: [1, 2, 3],
    nested: {
        flag: true,
        data: null
    }
};

"recursive generic type aliases work";
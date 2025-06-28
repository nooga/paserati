// Test forward reference resolution issue
type Node = { value: number; children: Node[] };

let tree: Node = { 
    value: 1, 
    children: [
        { value: 2, children: [] },
        { value: 3, children: [{ value: 4, children: [] }] }
    ]
};
tree.value;
// expect: 1
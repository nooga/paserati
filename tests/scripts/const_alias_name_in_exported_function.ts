// expect: 6
// A const in an exported function that shares its name with a const
// elsewhere is not taken for that one's alias when it narrows (#709).
function read(store: { size: number }): number {
    const item = store.size;
    return item;
}

export function check(n: number): number {
    const item = n;
    if (item) return 1;
    return 0;
}

read({ size: 5 }) + check(1);

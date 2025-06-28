// Simple recursive type test
type SimpleList = { value: number; next?: SimpleList };

let list: SimpleList = { value: 1, next: { value: 2 } };
list.value;
// expect: 1
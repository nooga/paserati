// Test recursive type assignment compatibility
type SimpleList = { value: number; next?: SimpleList };

let list: SimpleList = { 
    value: 1, 
    next: { 
        value: 2, 
        next: { value: 3 } 
    } 
};
list.value;
// expect: 1
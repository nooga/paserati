// expect: simple recursive type works

// Very simple test first
type SimpleList = {
    value: number;
    next?: SimpleList;
};

let list: SimpleList = {
    value: 1,
    next: {
        value: 2
    }
};

"simple recursive type works";
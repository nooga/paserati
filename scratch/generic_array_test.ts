// Test generic functions with arrays

function map<T, U>(arr: Array<T>, fn: (x: T) => U): Array<U> {
    let result: Array<U> = [];
    for (let item of arr) {
        result.push(fn(item));
    }
    return result;
}

let doubled = map([1, 2, 3], x => x * 2);
doubled;

// expect: 2,4,6
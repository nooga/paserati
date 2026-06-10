// expect: 1
type Result = { done: true; value: 1 } | { done: false; value: 2 };

function next(): Result {
    return { done: true, value: 1 };
}

let result: Result;
if ((result = next()).done) {
    let value: 1 = result.value;
}

1;

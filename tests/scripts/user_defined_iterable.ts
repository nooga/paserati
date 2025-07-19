// Test user-defined iterable with Symbol.iterator
// expect: done

// Create a custom iterable object
let customIterable = {
    [Symbol.iterator]() {
        let index = 0;
        let data = ["apple", "banana", "cherry"];
        
        return {
            next() {
                if (index < data.length) {
                    return { value: data[index++], done: false };
                } else {
                    return { value: undefined, done: true };
                }
            }
        };
    }
};

// Test that it's properly typed as Iterable
let typed: Iterable<string> = customIterable;

// Test for...of loop with user-defined iterable
for (let fruit of customIterable) {
    console.log("fruit:", fruit);
}

// Test manual iterator usage
let iterator = customIterable[Symbol.iterator]();
let result = iterator.next();
while (!result.done) {
    console.log("manual:", result.value);
    result = iterator.next();
}

"done";
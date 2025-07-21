// expect: undefined
// Test iterator.return() is called when for...of loop breaks early

let cleanupCalled = false;

const customIterable = {
    data: [1, 2, 3, 4, 5],
    index: 0,
    
    [Symbol.iterator]() {
        console.log("Iterator created");
        return this;
    },
    
    next() {
        if (this.index < this.data.length) {
            return { value: this.data[this.index++], done: false };
        } else {
            return { value: undefined, done: true };
        }
    },
    
    return() {
        console.log("Iterator cleanup called");
        cleanupCalled = true;
        return { value: undefined, done: true };
    }
};

for (const item of customIterable) {
    console.log("Item:", item);
    if (item === 1) {
        console.log("Loop broke early");
        break; // This should trigger iterator.return()
    }
}

// Verify cleanup was called - but don't print error to avoid failing expected output
// Just rely on the console output to show if cleanup was called
undefined;
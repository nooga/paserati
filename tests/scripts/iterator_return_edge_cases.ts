// expect: done
// Test iterator.return() edge cases

let returnCalledCount = 0;

// Test 1: Iterator without return method (shouldn't crash)
const iteratorWithoutReturn = {
    data: [1, 2],
    index: 0,
    
    [Symbol.iterator]() {
        return this;
    },
    
    next() {
        if (this.index < this.data.length) {
            return { value: this.data[this.index++], done: false };
        }
        return { value: undefined, done: true };
    }
    // No return method - should not crash
};

for (const item of iteratorWithoutReturn) {
    if (item === 1) break;
}

// Test 2: Iterator with return method that gets called
const iteratorWithReturn = {
    data: [1, 2],
    index: 0,
    
    [Symbol.iterator]() {
        return this;
    },
    
    next() {
        if (this.index < this.data.length) {
            return { value: this.data[this.index++], done: false };
        }
        return { value: undefined, done: true };
    },
    
    return() {
        returnCalledCount++;
        return { value: undefined, done: true };
    }
};

for (const item of iteratorWithReturn) {
    if (item === 1) break;
}

// Test 3: Loop that completes normally shouldn't call return()
const iteratorCompleteNormally = {
    data: [1],
    index: 0,
    
    [Symbol.iterator]() {
        return this;
    },
    
    next() {
        if (this.index < this.data.length) {
            return { value: this.data[this.index++], done: false };
        }
        return { value: undefined, done: true };
    },
    
    return() {
        returnCalledCount++; // This should NOT be called
        return { value: undefined, done: true };
    }
};

for (const item of iteratorCompleteNormally) {
    // Let loop complete normally
}

// Should have called return only once (from test 2)
console.log("Return called count:", returnCalledCount);

if (returnCalledCount === 1) {
    console.log("SUCCESS: iterator.return() called exactly once for early break");
} else {
    console.log("ERROR: expected 1 call to return(), got", returnCalledCount);
}

"done";
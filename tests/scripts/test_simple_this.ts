// Simple test of explicit this parameter

// expect: undefined

function Test(this: { value: number }, x: number) {
    return this.value + x;
}

console.log("Explicit this parameter is working!")
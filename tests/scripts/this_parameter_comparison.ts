// Test comparing explicit this parameter vs regular function

// expect: {explicit: "From explicit this", regular: undefined}

function ExplicitThis(this: { value: string }) {
    return this.value;
}

function RegularFunction() {
    return this;
}

// Test constructor usage
function TestConstructor(this: { explicit: string; regular: any }) {
    this.explicit = "From explicit this";
    this.regular = RegularFunction();
}

let result = new TestConstructor();
result
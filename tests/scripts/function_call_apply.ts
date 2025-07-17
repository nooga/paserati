// Test Function.prototype.call and apply methods
// expect: 47

function add(x: number, y: number): number {
    return x + y;
}

function multiply(x: number, y: number): number {
    return x * y;
}

function getThis(): any {
    return this;
}

let obj = {
    value: 10,
    getValue: function() {
        return this.value;
    }
};

// Test basic call
let result1 = add.call(null, 3, 4); // 7

// Test basic apply  
let result2 = add.apply(null, [5, 6]); // 11

// Test this binding with call
let result3 = obj.getValue.call({value: 20}); // 20

// Test this binding with apply
let result4 = obj.getValue.apply({value: 9}, []); // 9

// Sum all results
result1 + result2 + result3 + result4; // 7 + 11 + 20 + 9 = 47
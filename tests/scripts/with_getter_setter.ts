// Test with statement with getters/setters
// FIXME: Getters/setters in with statements not yet implemented
// expect_runtime_error: Operands must be two numbers

let obj = {
    _value: 42,
    get value() {
        return this._value;
    },
    set value(v) {
        this._value = v;
    }
};

with (obj) {
    value = value * 2; // Should trigger setter and getter
}
obj.value;
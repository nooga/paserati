// Test with statement with getters/setters
// expect: 84

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
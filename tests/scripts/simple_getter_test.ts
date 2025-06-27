class Test {
    private _value: string;
    
    constructor(value: string) {
        this._value = value;
    }
    
    get value(): string {
        return this._value;
    }
}

let obj = new Test("hello");
obj.value;
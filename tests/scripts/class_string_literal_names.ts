class StringLiteralClass {
    "fff" = 10;
    "eee": boolean = true;
    
    "aaa"() {
        return true;
    }
    
    get "bbb"() {
        return true;
    }
    
    set "ccc"(value: boolean) {
        // setter implementation
    }
    
    static "ddd" = "static property";
    
    static "ggg"() {
        return "static method";
    }
}

let obj = new StringLiteralClass();

// expect: true
obj["aaa"]();
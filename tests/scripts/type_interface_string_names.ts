type MyType = {
    "aaa"(): boolean;
    "yyy": boolean;
};

interface MyInterface {
    "yyy": boolean;
    "aaa"(): boolean;
}

let obj: MyType = {
    "aaa"() {
        return true;
    },
    "yyy": true
};

// expect: true
obj["aaa"]();
const obj = {
    "aaa"() {
        return true;
    },
    "eeee": true,
    "fff": 10
};

// expect: true
obj["aaa"]();
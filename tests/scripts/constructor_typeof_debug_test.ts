// expect: constructor typeof debug test

// Debug typeof for constructor to see structure

class SimpleClass {
    constructor(name: string) {}
}

function test() {
    // Check what typeof gives us for a class
    type ClassType = typeof SimpleClass;
    
    // This should compile
    let classRef: ClassType = SimpleClass;
    
    return "success";
}

test();

"constructor typeof debug test";
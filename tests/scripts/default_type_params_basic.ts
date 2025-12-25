// Test basic default type parameters - should parse successfully now

function test<T = string>() {
    return "hello";
}

interface Container<T = number> {
    value: T;
}

type Alias<T = boolean> = T;

class MyClass<T = string> {
    value: T;
}

const arrow = <T = number>() => {
    return 42;
};

// Test combined constraint and default: T extends BaseType = DefaultType
function complexTest<T extends object = { prop: string }>() {
    return {};
}

// expect: undefined
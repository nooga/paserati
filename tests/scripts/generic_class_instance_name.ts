// expect_compile_error: Type 'Box<string>' is not assignable to type 'number'.
class Box<T> {
    constructor(readonly value: T) {}
    get(): T { return this.value; }
    map(f: (v: T) => T): Box<T> { return this; }
}
const n: number = new Box<string>("x");

// expect_compile_error: Object literal may only specify known properties, but 'itm' does not exist in type '{ item: string }'.
class X {
    a(): void {}
}
class T0 {
    make(cls: new () => X, def: { item: string }): void;
    make(name: string, def: { item: string }): void;
    make(x: any, def: any): any {}
}
new T0().make(X, { itm: "x" });

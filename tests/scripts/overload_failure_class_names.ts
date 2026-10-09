// expect_compile_error: Argument of type 'typeof X' is not assignable to parameter of type 'string'.
class X {
    a(): void {}
}
class T0 {
    make(cls: new () => X, def: { item: string }): void;
    make(name: string, def: { item: string }): void;
    make(x: any, def: any): any {}
}
new T0().make(X, { itm: "x" });

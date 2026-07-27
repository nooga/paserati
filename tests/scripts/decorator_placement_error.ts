// expect_compile_error: Decorators are not valid here.
// TS1206: decorators attach to classes and class elements only, so a
// decorated enum, interface, type alias or variable is an error.

declare function dec<T>(target: T): T;

@dec
enum E {
    A,
}

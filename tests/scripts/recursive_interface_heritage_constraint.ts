// A constraint naming an interface still being declared is checked once it is
// complete: BB lacks y, CC has it.
// expect_compile_error: Type 'BB' does not satisfy the constraint '{ y: number }'.
interface AA<T extends { y: number }> { x: T }
interface BB extends AA<BB> {}
interface CC extends AA<CC> { y: number }

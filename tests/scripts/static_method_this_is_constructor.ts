// expect_compile_error: Property 'x' does not exist on type 'typeof C'
class C { x = 1; static f(): number { return this.x; } }

// ?? cannot be mixed with || or && without parentheses.
// expect_compile_error: '??' cannot be mixed with '||' or '&&'

let a: any = null, b: any = 1, c: any = 2;
let r = a ?? b || c;

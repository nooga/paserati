// expect_compile_error: 'await' expressions are only allowed within async functions
function f(): number { const x = await Promise.resolve(1); return x; }

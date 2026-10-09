// expect_compile_error: Class 'P' incorrectly implements interface 'N'
interface N { name: string; greet(): string }
class P implements N { name = "x"; }

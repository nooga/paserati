// expect_compile_error: Class 'Base' used before its declaration.
// `extends` evaluates immediately, so the base class must come first.

class Derived extends Base {}
class Base {}

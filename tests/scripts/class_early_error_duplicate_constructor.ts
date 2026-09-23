// A class may have only one constructor
class C {
  constructor() {}
  constructor(a: number) {}
}

// expect_compile_error: A class may only have one constructor

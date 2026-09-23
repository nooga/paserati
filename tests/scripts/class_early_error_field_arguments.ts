// Field initializers may not reference arguments, even through arrows
class C {
  x = () => {
    const t = () => arguments;
    return t;
  };
}

// expect_compile_error: 'arguments' is not allowed in class field initializer

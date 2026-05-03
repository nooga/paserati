// expect_compile_error: secret

namespace N {
  const secret = 99;
  export const visible = 5;
}

N.secret;

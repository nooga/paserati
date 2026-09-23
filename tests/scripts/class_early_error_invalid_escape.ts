// An identifier escape must denote an ID_Start/ID_Continue code point
class C {
  #\u0000;
}

// expect_compile_error: expected identifier

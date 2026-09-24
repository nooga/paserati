// expect_compile_error: Identifier 'a' has already been declared
// A var hoists through the block that holds the let of the same name.
{
  let a = 1;
  {
    var a = 2;
  }
}

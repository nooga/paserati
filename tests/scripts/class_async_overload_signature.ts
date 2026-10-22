// expect_compile_error: Argument of type 'string' is not assignable to parameter of type 'number'
class Loader {
  async load(id: number): Promise<number>;
  async load(id: any): Promise<any> { return id; }
}
new Loader().load("x");

// expect_compile_error: Overload 2 of 2, '(a: number, b: number): string', gave the following error.
class T {
  m(a: string, b: string): number;
  m(a: number, b: number): string;
  m(a: any, b: any): any { return a; }
}
new T().m(true, 1);

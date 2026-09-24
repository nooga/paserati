// expect: 3
declare module "some-lib" {
  export function helper(p: string): string;
  interface Options { a: number }
}
declare global {
  interface GlobalThing { b: number }
}
let v = 3;
v;

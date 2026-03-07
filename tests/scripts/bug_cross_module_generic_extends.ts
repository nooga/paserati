// expect: ok
// Regression test: cross-module generic class extends should not panic the checker
// The checker previously panicked with: interface conversion: types.Type is *types.Primitive, not *types.GenericType
import { Base } from "./bug_cross_module_generic_extends_helper.ts";

interface MyState { x: number }
interface MyResult { y: string }

class Child extends Base<MyState, MyResult> {
  step(s: MyState): MyState { return { x: s.x + 1 }; }
  finalize(s: MyState): MyResult { return { y: "done " + s.x }; }
}

"ok";

// expect: true false true
// A method or getter returning its own generic class works on an instance
// imported from another module (#704): a fluent expect(x).not.is(y).
import { e } from "./self_returning_generic_method_cross_module_helper.ts";

`${e("a").not.is("b")} ${e("a").negate().is("a")} ${e(1).not.not.is(1)}`;

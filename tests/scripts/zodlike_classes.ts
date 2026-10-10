// A generic class is a type through a namespace import (z.Base<any>), a
// subclass is assignable to its base instantiated with any, `this` in a
// member's type is the receiver, and Wrap<this> resolves in the module that
// declared it, not the one calling opt().
import * as z from "./zodlike_classes_helper.ts";
function check(s: z.Str): number {
  const b: z.Base<any> = s;
  const bare: z.Base = s;
  const w: z.Wrap<z.Str> = s.opt();
  const o: string | undefined = s.opt()._o;
  return 1;
}
"ok";

// expect: ok

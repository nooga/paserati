// expect: var_0|var_1|7|8|2|3|k|number|base|3
// #192: a hoisted function declaration in an imported module referencing that
// module's own top-level let/class. A plain read already resolved to the
// module-namespaced heap slot (#117), but ++/--, destructuring assignment,
// for-of/for-in targets, typeof and `class extends` still allocated the slot
// under the bare name, so typebox's Unique() threw "index is not defined".
import { Unique } from "./module_hoisted_ref/counter.ts";
import { destructure, loops, kind, derived, bump } from "./module_hoisted_ref/targets.ts";

`${Unique()}|${Unique()}|${destructure()}|${loops()}|${kind()}|${derived()}|${bump()}`;

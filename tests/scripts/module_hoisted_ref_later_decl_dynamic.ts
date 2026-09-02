// expect: var_0|var_1|7|8|2|3|k|number|base|3
// Dynamic-import form of module_hoisted_ref_later_decl.ts, the shape #192 was
// reported with (typebox and typebox/compile reached via import()).
const { Unique } = await import("./module_hoisted_ref/counter.ts");
const { destructure, loops, kind, derived, bump } = await import("./module_hoisted_ref/targets.ts");

`${Unique()}|${Unique()}|${destructure()}|${loops()}|${kind()}|${derived()}|${bump()}`;

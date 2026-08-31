// expect: 0,1,2

// #132: a class declared directly inside a loop body must also get a fresh
// binding each iteration. Local class name bindings always live in a spill
// slot rather than a register (#128), so this needs its own per-iteration
// close mechanism (OpCloseUpvalueSpill) alongside the register-based one
// used for plain let/const. Before this fix, all three closures shared the
// loop's single spill slot and returned [2, 2, 2].
function f(): number[] {
    let out: (() => number)[] = [];
    for (let i = 0; i < 3; i++) {
        class C {
            static id() {
                return i;
            }
        }
        out.push(() => C.id());
    }
    return out.map(g => g());
}

f().join(",");

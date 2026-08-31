// expect: for=0,10,20 while=0,10,20 doWhile=0,10,20 forOf=0,10,20 forIn=0,10,20 forContinue=10,30 whileContinue=10,30,50 nested=0,1,10,11 siblingIfElse=1,2 labeledContinue=10,20,30,40,50 forConstBody=0,10,20

// #132: per-iteration bindings for a body-declared let must hold for every
// loop construct that repeats its body (for, while, do-while, for-of,
// for-in), survive a `continue` (which must still close the current
// iteration's binding before moving on - it must not skip straight past the
// close the way it skips the update/increment), and nest correctly (an inner
// loop's body-local must not be attributed to the outer loop's per-iteration
// list, and vice versa). Also checks that ordinary (non-loop) sibling blocks
// reusing a name - never broken by this issue, but worth pinning down as a
// non-regression - still resolve correctly.

function testFor(): string {
    let out: (() => number)[] = [];
    for (let i = 0; i < 3; i++) {
        let x = i * 10;
        out.push(() => x);
    }
    return out.map(g => g()).join(",");
}

function testWhile(): string {
    let out: (() => number)[] = [];
    let i = 0;
    while (i < 3) {
        let x = i * 10;
        out.push(() => x);
        i++;
    }
    return out.map(g => g()).join(",");
}

function testDoWhile(): string {
    let out: (() => number)[] = [];
    let i = 0;
    do {
        let x = i * 10;
        out.push(() => x);
        i++;
    } while (i < 3);
    return out.map(g => g()).join(",");
}

function testForOf(): string {
    let out: (() => number)[] = [];
    for (const i of [0, 1, 2]) {
        let x = i * 10;
        out.push(() => x);
    }
    return out.map(g => g()).join(",");
}

function testForIn(): string {
    let out: (() => number)[] = [];
    let obj: Record<string, number> = { a: 0, b: 1, c: 2 };
    for (const k in obj) {
        let x = obj[k] * 10;
        out.push(() => x);
    }
    return out.map(g => g()).join(",");
}

function testForContinue(): string {
    let out: (() => number)[] = [];
    for (let i = 0; i < 5; i++) {
        if (i % 2 === 0) continue;
        let x = i * 10;
        out.push(() => x);
    }
    return out.map(g => g()).join(",");
}

function testWhileContinue(): string {
    let out: (() => number)[] = [];
    let i = 0;
    while (i < 5) {
        i++;
        if (i % 2 === 0) continue;
        let x = i * 10;
        out.push(() => x);
    }
    return out.map(g => g()).join(",");
}

function testNestedLoop(): string {
    let out: (() => number)[] = [];
    for (let i = 0; i < 2; i++) {
        for (let j = 0; j < 2; j++) {
            let x = i * 10 + j;
            out.push(() => x);
        }
    }
    return out.map(g => g()).join(",");
}

function testSiblingIfElse(): string {
    function pick(flag: boolean): () => number {
        if (flag) {
            let x = 1;
            return () => x;
        } else {
            let x = 2;
            return () => x;
        }
    }
    return [pick(true)(), pick(false)()].join(",");
}

function testLabeledContinue(): string {
    // A labeled continue on an OUTER while must land at the same
    // post-close, pre-jump-back position as an unlabeled one - it goes
    // through the same ContinuePlaceholderPosList, just resolved by label
    // instead of "nearest enclosing loop" (see compileContinueStatement).
    let out: (() => number)[] = [];
    let i = 0;
    outer: while (i < 5) {
        i++;
        let x = i * 10;
        out.push(() => x);
        if (i % 2 === 0) continue outer;
    }
    return out.map(g => g()).join(",");
}

function testForConstBody(): string {
    // Exercises compileConstStatement's tracking hooks specifically -
    // testFor above only exercises compileLetStatement's.
    let out: (() => number)[] = [];
    for (let i = 0; i < 3; i++) {
        const x = i * 10;
        out.push(() => x);
    }
    return out.map(g => g()).join(",");
}

[
    "for=" + testFor(),
    "while=" + testWhile(),
    "doWhile=" + testDoWhile(),
    "forOf=" + testForOf(),
    "forIn=" + testForIn(),
    "forContinue=" + testForContinue(),
    "whileContinue=" + testWhileContinue(),
    "nested=" + testNestedLoop(),
    "siblingIfElse=" + testSiblingIfElse(),
    "labeledContinue=" + testLabeledContinue(),
    "forConstBody=" + testForConstBody(),
].join(" ");

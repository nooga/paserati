// expect: ok
// The contextual-keyword lookahead for `using` in a for-loop head must not
// claim heads that merely start with `await` or with a variable named `using`.

const log: string[] = [];

function res(name: string) {
    return { [Symbol.dispose]() { log.push(name); } };
}

async function awaitExpressionInHead(): Promise<void> {
    const foo = 1;
    // `await foo` here is an expression, not an `await using` declaration.
    for (await foo; false; ) {
    }
}

function usingAsIterationVariable(): void {
    // `using` is an ordinary variable being iterated over, not a declaration.
    let using: number;
    for (using of [1, 2]) {
        log.push("of" + using);
    }
}

function usingDeclarations(): void {
    for (using d of [res("a")]) {
    }
    for (using x = res("b"); ; ) {
        break;
    }
}

awaitExpressionInHead();
usingAsIterationVariable();
usingDeclarations();

log.join(",") === "of1,of2,a,b" ? "ok" : "got " + log.join(",");

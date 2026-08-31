// expect: 0,10,20

// #132: a `let` declared directly in a `for` loop's body must get a fresh
// binding each iteration, so closures created in different iterations each
// capture their own iteration's value - the classic reason `let` was added
// to the language over `var`. Before this fix, all three closures shared the
// loop's single, final register/binding and returned [2, 2, 2].
function f(): number[] {
    let out: (() => number)[] = [];
    for (let i = 0; i < 3; i++) {
        let x = i * 10;
        out.push(() => x);
    }
    return out.map(g => g());
}

f().join(",");

// expect: ok
function v(d: string) {
    if (d) {
        const re = /a/;
        if (!re.test(d)) {
            throw new Error("x");
        }
    }
}
v("a");
"ok";

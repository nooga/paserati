// expect: a
let out = "";
function f(src: string): void {
    const items = [];
    items.push(src.substring(0, 1));
    items.forEach(item => {
        const m = /a/.exec(item);
        if (!m) return;
        out = m[0];
    });
}
f("abc");
out;

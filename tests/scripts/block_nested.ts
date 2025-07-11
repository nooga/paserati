// Nested block statements

let outer = 1;
{
    let middle = 2;
    {
        let inner = 3;
        outer = inner;
    }
    outer = outer + middle;
}
outer;
// expect: 5
// Block statements have no value (execute for side effects only)

let result: number | null = null;
{
    let x = 42;
    result = x;
}
// The block itself has no value, but it modifies result
result;
// expect: 42
// expect: 42
// Bug fix: export async function should parse correctly
export async function foo() {
    return 42;
}
const result = await foo();
result;

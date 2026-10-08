// expect_compile_error: Cannot assign to 'id' because it is a read-only property
interface User { readonly id: string; name: string }
const u: User = { id: "1", name: "a" };
u.name = "b";
u.id = "2";

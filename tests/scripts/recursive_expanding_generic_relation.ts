// expect: 3
// Infinitely expanding generics (each level nests the type in itself) must
// relate structurally without walking forever.
interface IL<T> { data: T; next: IL<IL<T>> | null; }
interface IM<T> { data: T; next: IM<IM<T>> | null; }
class CL<T> { data: T; next: CL<CL<T>> | null = null; constructor(d: T) { this.data = d; } }
class CM<T> { data: T; next: CM<CM<T>> | null = null; constructor(d: T) { this.data = d; } }
const im: IM<number> = { data: 1, next: null };
const il: IL<number> = im;
const cl: CL<number> = new CM(2);
il.data + cl.data;

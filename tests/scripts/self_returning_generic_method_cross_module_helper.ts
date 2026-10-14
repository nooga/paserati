export class E<T> {
    constructor(private readonly v: T, private readonly neg: boolean = false) {}
    get not(): E<T> { return new E(this.v, !this.neg); }
    negate(): E<T> { return new E(this.v, !this.neg); }
    is(x: T): boolean { return (this.v === x) !== this.neg; }
}
export function e<T>(v: T): E<T> { return new E(v); }

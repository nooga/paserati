// Declared like zod: Base names Wrap before Wrap is declared, and returns
// the polymorphic this.
export declare class Base<O = any> { readonly _o: O; opt(): Wrap<this>; }
export declare class Wrap<T extends Base<any>> extends Base<T["_o"] | undefined> {}
export declare class Str extends Base<string> {}
export declare function str(): Str;

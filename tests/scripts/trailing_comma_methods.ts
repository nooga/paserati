// Test trailing commas in method type signatures

// Interface with trailing comma in method signature
interface Test1 {
    method1(value: string,): void;
    method2(a: number, b: string,): boolean;
    method3(x: any, y: any, z: any,): number;
}

// Interface with mixed trailing commas
interface Test2 {
    // No trailing comma
    methodA(value: string): void;
    
    // With trailing comma
    methodB(value: string,): void;
    
    // Multiple params with trailing comma
    methodC(a: number, b: string, c: boolean,): void;
    
    // Complex type with trailing comma
    methodD(value: string & {},): void;
}

// Test with optional parameters and trailing comma
interface Test3 {
    optional1(value?: string,): void;
    optional2(a: number, b?: string,): void;
    optional3(a?: number, b?: string, c?: boolean,): void;
}

// Test with generic interface (but not generic methods)
interface Test4<T> {
    method1(value: T,): void;
    method2(a: T, b: T,): T;
}

// Test empty parameter list (no trailing comma possible)
interface Test5 {
    empty(): void;
}

// Test regular property (not computed method)
interface Test6 {
    prop: string;
}

// Implementation to verify it compiles
class TestImpl implements Test1 {
    method1(value: string): void {}
    method2(a: number, b: string): boolean { return true; }
    method3(x: any, y: any, z: any): number { return 42; }
}

console.log("ok");
"ok"; // Return value to match expectation

// expect: ok
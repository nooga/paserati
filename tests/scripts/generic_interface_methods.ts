// Generic method signatures in interfaces - with class implementation
// expect: transformed:42

// Interface with generic method signature (not named `Transformer`: in a
// script that would merge with lib.dom's Web Streams Transformer<I, O>)
interface ValueTransformer {
  transform<T, U>(input: T, fn: (x: T) => U): U;
  identity<T>(x: T): T;
}

// Class implementing interface with generic methods
class MyTransformer implements ValueTransformer {
  transform<T, U>(input: T, fn: (x: T) => U): U {
    return fn(input);
  }
  identity<T>(x: T): T {
    return x;
  }
}

const transformer: ValueTransformer = new MyTransformer();
transformer.transform(42, (n: number) => "transformed:" + n);

// Generic method signatures in interfaces - with class implementation
// expect: transformed:42

// Interface with generic method signature
interface Transformer {
  transform<T, U>(input: T, fn: (x: T) => U): U;
  identity<T>(x: T): T;
}

// Class implementing interface with generic methods
class MyTransformer implements Transformer {
  transform<T, U>(input: T, fn: (x: T) => U): U {
    return fn(input);
  }
  identity<T>(x: T): T {
    return x;
  }
}

const transformer: Transformer = new MyTransformer();
transformer.transform(42, (n: number) => "transformed:" + n);

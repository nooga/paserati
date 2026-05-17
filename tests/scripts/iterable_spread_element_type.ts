// expect_compile_error: Type 'symbol' is not assignable to type 'number'
class SymbolIteratorForSpread {
  next() {
    return {
      value: Symbol(),
      done: false,
    };
  }

  [Symbol.iterator]() {
    return this;
  }
}

let values: number[] = [...new SymbolIteratorForSpread()];

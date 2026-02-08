# Paserati Bugs (Minimal Repros)

This file documents Paserati issues found while integrating, but each repro is a standalone TypeScript snippet that does not depend on Anjin.

## 1. Derived Class `super()` Called Twice

**Symptom**: Runtime error like `super() already called` when using a normal derived class constructor or field initializers.

**Minimal repro**:
```ts
class Base {
  x: number;
  constructor() {
    this.x = 1;
  }
}

class Child extends Base {
  constructor() {
    super();
  }
}

export default Child;
```

**Expected**: `new Child()` succeeds.

**Actual**: Runtime error indicating `super()` was already called. This suggests Paserati may be injecting a `super()` call for derived classes, so user code calling `super()` (or using field initializers that implicitly call `super()`) triggers a double call.

## 2. Callables Misclassified as Non-Functions

**Symptom**: Exports that are callable report `TypeName() == "function"`, but `IsFunction()` returns false. Only `IsCallable()` detects them. This breaks code that uses `IsFunction()` to detect export types.

**Minimal repro**:
```ts
export function f(): number { return 1; }
export default class C { }
```

**Expected**: runtime function checks succeed for both `f` and `C`.

**Actual**: some callables are not recognized by `IsFunction()` despite being callable. `IsCallable()` works.

## 3. Debug Type-Checker Output Leaks to Stdout

**Symptom**: `DEBUG Return check: ...` lines printed during normal module execution.

**Minimal repro**:
```ts
export function id<T>(x: T): T { return x; }
```

**Expected**: no debug output by default.

**Actual**: debug lines are printed even in non-debug runs.

## 4. Unsupported TS Type Syntax (Variadic Tuple / Rest)

**Symptom**: Parser or checker errors for valid TS type syntax involving variadic tuples/rests.

**Minimal repro (variadic tuple)**:
```ts
type Fn = (...args: [...string[]]) => void;
```

**Minimal repro (rest element)**:
```ts
type Fn = (...args: [string, ...string[]]) => void;
```

**Expected**: valid TypeScript type syntax.

**Actual**: parse or type-check errors in Paserati.

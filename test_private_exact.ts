class C {
  #setterCalledWith: any;

  get #field() {
    console.log("getter called");
    return null;
  }

  set #field(value: any) {
    console.log("setter called with:", value);
    this.#setterCalledWith = value;
  }

  compoundAssignment() {
    const result = (this.#field ??= 1);
    console.log("compound assignment result:", result);
    return result;
  }

  setterCalledWithValue() {
    console.log("getting setterCalledWith...");
    return this.#setterCalledWith;
  }
}

const o = new C();
console.log("=== Starting test ===");
const result = o.compoundAssignment();
console.log("=== After compound assignment ===");
console.log("result:", result);
console.log("=== Getting setter value ===");
const setterValue = o.setterCalledWithValue();
console.log("setterCalledWith:", setterValue);

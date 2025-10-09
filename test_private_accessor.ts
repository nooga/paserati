class C {
  #setterCalledWith: any;

  get #field() {
    return null;
  }

  set #field(value: any) {
    this.#setterCalledWith = value;
  }

  compoundAssignment() {
    return this.#field ??= 1;
  }

  setterCalledWithValue() {
    return this.#setterCalledWith;
  }
}

const o = new C();
const result = o.compoundAssignment();
console.log("result:", result);
console.log("setterCalledWith:", o.setterCalledWithValue());

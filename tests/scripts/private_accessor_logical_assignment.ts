// expect: 1
class C {
  #backingField: any;

  get #field() {
    return null;
  }

  set #field(value: any) {
    this.#backingField = value;
  }

  test() {
    return this.#field ??= 1;
  }

  getBackingField() {
    return this.#backingField;
  }
}

const o = new C();
o.test();

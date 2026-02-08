class Base {
  constructor() {
    this.list = this.list.bind(this);
  }
  list(): string[] {
    return [];
  }
}

class Child extends Base {
  kv: Record<string, unknown> = {};
  constructor() {
    super();
  }
}

export default Child;

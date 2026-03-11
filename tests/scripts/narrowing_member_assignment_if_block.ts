// expect: ["a", "b"]
// Test: member expression assignment in if-block narrows property type
// if (this._tools === null) { this._tools = [...]; } should narrow to non-null

class Container {
  private _items: string[] | null = null;

  get items(): string[] {
    if (this._items === null) {
      this._items = ["a", "b"];
    }
    return this._items;
  }
}

const c = new Container();
c.items;

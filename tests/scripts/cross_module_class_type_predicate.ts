// expect: 42
// Cross-module class inheritance with type predicate narrowing in methods
import { BaseReader, ValueNode } from "./cross_module_class_type_predicate_helper.ts";

class MyReader extends BaseReader {
  constructor() {
    super();
    this.nodes = {
      answer: { kind: "value", get: () => 42 } as ValueNode,
    };
  }
}

const reader = new MyReader();
reader.read("answer");

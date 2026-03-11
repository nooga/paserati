// expect: 42
// Cross-module test: type predicate narrowing with mapped types
// Emulates the anjin driver pattern where type predicates and
// mapped types defined in a module are used for sequential narrowing.
import { NodeMap, ValueNode, readNode } from "./cross_module_type_predicate_narrowing_helper.ts";

const nodes: NodeMap = {
  answer: { kind: "value", get: () => 42 } as ValueNode,
  greeting: { kind: "value", value: "hello" } as ValueNode,
};

readNode(nodes, "answer");

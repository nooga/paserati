import { Base } from "./bug_abstract_class_helper.ts";

class Child extends Base {
  step(): string { return "hello"; }
}

const c = new Child();
c.step();

// expect: hello

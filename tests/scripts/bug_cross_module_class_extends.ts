// expect: woof
// Bug 2: non-generic cross-module class extends should work
import { Animal } from "./bug_cross_module_class_extends_helper.ts";

class Dog extends Animal {
  speak(): string {
    return "woof";
  }
}

const d = new Dog();
d.speak();

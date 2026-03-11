// Test extending a generic class and accessing inherited methods
// Reproduces the anjin pattern: class MyAgent extends Agent<any, any>

import { Agent } from "./decorator_generic_base_class_helper.ts";

class MyAgent extends Agent<any, any> {
  systemPrompt: string = "test";

  step(state: any): any {
    const tools = this.tools();
    return tools.length;
  }

  initialize(event: any): any { return {}; }
  finalize(state: any): any { return state; }
}

const a = new MyAgent();
a.step({});

// expect: 0

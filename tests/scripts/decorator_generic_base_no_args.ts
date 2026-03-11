// Test extending a generic class WITHOUT type arguments
// This is the most common pattern in anjin: class MyAgent extends Agent

import { Agent } from "./decorator_generic_base_class_helper.ts";

class MyAgent extends Agent {
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

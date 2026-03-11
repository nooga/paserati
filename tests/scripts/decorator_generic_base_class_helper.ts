interface ToolDef {
  name: string;
}

export abstract class Agent<StateType, ResultType> {
  systemPrompt: string = "";
  state: StateType = undefined as any;
  _tools: ToolDef[] = [];

  abstract step(state: StateType): StateType;
  abstract initialize(event: any): StateType;
  abstract finalize(state: StateType): ResultType;

  tools(): ToolDef[] {
    return this._tools;
  }

  stop(): void {}
}

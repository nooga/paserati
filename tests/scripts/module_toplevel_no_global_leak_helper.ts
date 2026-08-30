export function greet() {
  return "hi";
}

export let counter = 41;
export const PI_ISH = 3;
export var legacy = "old";

export default class Agent {
  n: number;
  constructor() {
    this.n = 7;
  }
  getN() {
    return this.n;
  }
}

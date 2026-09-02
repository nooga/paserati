import { transform, onlyInB } from "./_helper.ts";
export function useB(x: number): number { return transform(x); }
export function b(): string { return onlyInB(); }

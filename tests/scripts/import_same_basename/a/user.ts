import { transform, onlyInA } from "./_helper.ts";
export function useA(x: number): number { return transform(x); }
export function a(): string { return onlyInA(); }

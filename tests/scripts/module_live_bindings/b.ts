// skip-typecheck
export let x = 1;
let hidden = "h0";
export { hidden as renamed };
export var v = "v0";
export function f() { return "f0"; }
export class C { static tag = "c0"; }
export default function def() { return "d"; }
export function bump() { x += 10; hidden = "h1"; v = "v1"; f = () => "f1"; C = class { static tag = "c1"; }; }
export * as nsOfLive from "./live.ts";

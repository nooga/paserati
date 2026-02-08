// expect: ok
import Child from "./bug1_super_bind_helper.ts";

const c = new Child();
const hasList = typeof c.list === "function";
const hasKv = typeof c.kv === "object";
hasList && hasKv ? "ok" : "fail: list=" + typeof c.list + " kv=" + typeof c.kv;

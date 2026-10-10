// `export = e` (a function merged with a namespace, as @types/express) is the
// default import, and the namespace's members are named exports.
// expect_compile_error: Type 'number' is not assignable to type 'string'.
import ex from "./export_equals_helper.ts";
import { Router } from "./export_equals_helper.ts";
const app: ex.App = ex();
const r: string = Router();

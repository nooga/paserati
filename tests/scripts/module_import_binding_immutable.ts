// expect: TypeError,TypeError,TypeError,TypeError,42
// skip-typecheck
// Import bindings are immutable: every form of assignment to one throws a
// TypeError and leaves the binding unchanged.
import { value } from "./module_import_defer_helper.ts";
const r = [];
try { value = 1; } catch (e) { r.push(e.constructor.name); }
try { value++; } catch (e) { r.push(e.constructor.name); }
try { [value] = [1]; } catch (e) { r.push(e.constructor.name); }
try { value += 1; } catch (e) { r.push(e.constructor.name); }
r.push(value);
r.join(",");

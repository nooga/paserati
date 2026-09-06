// expect: dflt,dflt,dflt,dflt,dflt,dflt,namedFrom,namedFrom
// skip-typecheck
// isImportBindingName only special-cased `from`/`type`/`satisfies`, but any
// contextual keyword (see isContextualKeywordAsIdent) is a legal import
// binding name, not just those three: `async`, `get`, `set`, `of`, `static`,
// and `readonly` as default-import names, and `from` as a named-import
// shorthand and alias source, all used to fail with "Expected '{', '*', or
// an identifier after 'import'" / "Expected identifier, string literal, or
// 'default' in import specifier".
import async from "./import_binding_contextual_keywords_helper.ts";
import get from "./import_binding_contextual_keywords_helper.ts";
import set from "./import_binding_contextual_keywords_helper.ts";
import of from "./import_binding_contextual_keywords_helper.ts";
import static from "./import_binding_contextual_keywords_helper.ts";
import readonly from "./import_binding_contextual_keywords_helper.ts";
import { from } from "./import_binding_contextual_keywords_helper.ts";
import { from as fromAlias } from "./import_binding_contextual_keywords_helper.ts";

[async, get, set, of, static, readonly, from, fromAlias].join(",");

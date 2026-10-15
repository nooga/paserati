// expect: ok
// A type parameter defaulting to a type imported across an import cycle
// keeps its default: a bare Reply is not TS2707 (fastify's FastifyReply).
import type { RouteOptions } from "./cyclic_import_type_param_default_route.ts";
import type { Reply } from "./cyclic_import_type_param_default_reply.ts";
const r: Reply = { route: {}, x: "ok" };
const o: RouteOptions = { reply: r };
o.reply!.x;

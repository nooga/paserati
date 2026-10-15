import { RouteGeneric } from "./cyclic_import_type_param_default_route.ts";
export interface ReplyGeneric { reply?: string }
export interface Reply<R extends RouteGeneric = RouteGeneric, X = string> {
  route: R;
  x: X;
}

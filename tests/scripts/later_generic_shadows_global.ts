// expect: hi
// A module's own generic declared after its use shadows a global of the
// same name (express-serve-static-core's Request<P> vs the fetch Request).
declare global {
  interface Rq { url: string }
}
export interface Handler<P = string> {
  (req: Rq<P>): string;
}
export interface Rq<P = string> { params: P; }
const h: Handler = (req) => req.params;
h({ params: "hi" });

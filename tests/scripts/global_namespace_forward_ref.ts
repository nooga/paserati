// expect: 3
// A type naming a namespace that this module declares later inside
// `declare global` (@types/aws-lambda's awslambda.HttpResponseStream).
export type Handler<TEvent = any> = (event: TEvent, stream: ns1.Stream) => number;
declare global {
  namespace ns1 {
    interface Stream { size: number }
  }
}
const h: Handler<number> = (event, stream) => event + stream.size;
h(1, { size: 2 });

// expect: ok
type AnyConstructor = new (url: string) => object;
type EventOf<T extends AnyConstructor> = InstanceType<T>;
interface A<TConfig extends ConsumerConfig> {
  event: EventOf<TConfig['EventSource']>;
}
interface ConsumerConfig { EventSource: AnyConstructor; }
"ok";

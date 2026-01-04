// Advanced type inference with nested generics and callbacks
// expect: processed:hello:42

// Type-safe event handler system - now using native array syntax with parens!
type Handler = (data: [string, number]) => void;
type HandlerMap<E extends string> = { [K in E]?: ((data: [string, number]) => void)[] };

interface EventEmitter<E extends string> {
  handlers: HandlerMap<E>;
  on(event: E, handler: Handler): void;
  emit(event: E, data: [string, number]): void;
}

function createEmitter<E extends string>(): EventEmitter<E> {
  const handlers: { [key: string]: ((data: [string, number]) => void)[] } = {};

  return {
    handlers: handlers as HandlerMap<E>,
    on(event: E, handler: Handler): void {
      if (!handlers[event]) {
        handlers[event] = [];
      }
      handlers[event].push(handler);
    },
    emit(event: E, data: [string, number]): void {
      const eventHandlers = handlers[event];
      if (eventHandlers) {
        for (let i = 0; i < eventHandlers.length; i++) {
          eventHandlers[i](data);
        }
      }
    }
  };
}

// Create typed event emitter
const emitter = createEmitter<"click" | "hover">();

let result = "";

// Register handler with tuple parameter
emitter.on("click", (data: [string, number]) => {
  result = "processed:" + data[0] + ":" + data[1];
});

// Emit with tuple data
emitter.emit("click", ["hello", 42]);

result;

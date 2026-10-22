// expect: message
// A method's constraint naming the class's parameter (`K extends keyof T &
// string`) is read with the instance's type argument.
class Emitter<T> {
  on<K extends keyof T & string>(name: K): K {
    return name;
  }
}
interface Events {
  message: [text: string];
}
const emitter = new Emitter<Events>();
emitter.on("message");

// expect: initialized
let initialized = false;

function init(value: any, context: any) {
  context.addInitializer(function(this: any) {
    initialized = true;
  });
}

@init
class MyClass {}

initialized ? "initialized" : "not initialized";

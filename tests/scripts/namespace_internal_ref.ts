// expect: 100

namespace N {
  export let counter = 0;
  export function bump() {
    counter += 1;
  }
}

for (let i = 0; i < 100; i++) {
  N.bump();
}

N.counter;

// expect: 3-ok
function counter() {
  return counter.count++;
}
counter.count = 0;
counter.label = "ok";
const greet = () => "hi";
greet.times = 2;
counter();
counter();
counter();
counter.count + "-" + counter.label;

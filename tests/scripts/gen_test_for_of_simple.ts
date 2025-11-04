// Test for-of loop with Map
// expect: 4
var map = new Map();
map.set(0, 'a');
map.set(true, false);
map.set(null, undefined);
map.set(NaN, {});

var count = 0;
for (var x of map) {
  count += 1;
}
count;

function test(fn, str) {
  return fn(str);
}

test((s) => s.length, "hello");
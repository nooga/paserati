// A nested generator's closure binding that shares its name with an outer
// binding compiles and keeps the two apart (#85).
// expect: 5,15,function,2,1
function host(): number {
  function* g() {
    const get = () => 10;
    return get;
  }
  const get = 5;
  return get;
}
function host2(): number {
  const get = 5;
  function* g() {
    let get = () => 10;
    yield get;
  }
  return get + g().next().value();
}
function host3() {
  function* g() {
    const get = () => 10;
    yield get;
  }
  let get = g;
  return typeof get;
}
class K {
  *m() { const get = () => 1; yield get; }
  n() { const get = 2; return get; }
}
[host(), host2(), host3(), new K().n(), new K().m().next().value()].join(",");

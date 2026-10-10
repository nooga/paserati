declare function e(): e.App;
declare namespace e {
  interface App { listen(port: number): void }
  function Router(): number;
}
export = e;

// expect: 3
// An interface whose members index the interface itself (tailwind's
// ThemeConfig: `backdropBlur: ThemeConfig['blur']`) is checked in linear time.
type KeyValuePair<K extends keyof any = string, V = string> = Record<K, V>
type ResolvableTo<T> = T | ((utils: { theme(path: string): any }) => T)
interface Theme {
  c0: Theme['colors']
  c1: Theme['colors']
  c2: Theme['colors']
  c3: Theme['colors']
  c4: Theme['colors']
  c5: Theme['colors']
  c6: Theme['colors']
  c7: Theme['colors']
  opacity: ResolvableTo<KeyValuePair>
  o0: Theme['opacity']
  o1: Theme['opacity']
  o2: Theme['opacity']
  o3: Theme['opacity']
  o4: Theme['opacity']
  o5: Theme['opacity']
  o6: Theme['opacity']
  o7: Theme['opacity']
  colors: ResolvableTo<KeyValuePair>
}

const theme = { c3: { a: "1" }, o5: () => ({ b: "2", c: "3" }) } as Partial<Theme>;
const o5 = theme.o5;
typeof o5 === "function" ? Object.keys(o5({ theme: () => null })).length + 1 : 0;

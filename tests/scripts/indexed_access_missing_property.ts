// expect_compile_error: Property 'nope' does not exist on type 'Theme'.
// `T['k']` names a property T has: an interface indexing itself is checked
// once it is declared.
interface Theme {
  blur: string
  backdropBlur: Theme['blur']
  other: Theme['nope']
}

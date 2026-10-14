// expect: 1 b 1c 2
// A regex literal may start any template substitution, not just the first
// (#707); division inside one still lexes as division.
const x = 1, y = 4;
`${x} ${/b/.source} ${x}${/c/.source} ${y / 2}`;

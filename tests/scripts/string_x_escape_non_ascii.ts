// expect: true
// paserati#425: `\xNN` for NN in 0x80-0xFF names a Unicode code point
// (U+0080-U+00FF), not a raw byte - it must be UTF-8-encoded (2 bytes),
// not written as a single raw byte (which produces invalid UTF-8/WTF-8:
// a lone continuation-range byte). That corruption was invisible to
// charCodeAt()/length (which read logical UTF-16 code units) but broke
// anything that needs valid UTF-8 out of the string, like handing it to
// Go's regexp.Compile via `new RegExp(str)`.
const s = "\xaa";
const bytes = Array.from(new TextEncoder().encode(s));

bytes.length === 2 && bytes[0] === 0xc2 && bytes[1] === 0xaa && s.charCodeAt(0) === 0xaa;

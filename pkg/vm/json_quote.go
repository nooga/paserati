package vm

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// QuoteJSONString quotes and escapes a string for JSON output per ECMAScript
// QuoteJSONString, preserving lone surrogates as \uXXXX escapes. Go's
// json.Marshal is not a substitute: it replaces invalid UTF-8 (including lone
// surrogates) with U+FFFD, and HTML-escapes <, >, & and U+2028/U+2029, which
// the spec leaves as-is (paserati#516). Used for both keys and values.
func QuoteJSONString(s string) string {
	var buf strings.Builder
	buf.WriteByte('"')

	for i := 0; i < len(s); {
		b := s[i]
		// Check for characters that need escaping
		switch b {
		case '"':
			buf.WriteString("\\\"")
			i++
		case '\\':
			buf.WriteString("\\\\")
			i++
		case '\b':
			buf.WriteString("\\b")
			i++
		case '\f':
			buf.WriteString("\\f")
			i++
		case '\n':
			buf.WriteString("\\n")
			i++
		case '\r':
			buf.WriteString("\\r")
			i++
		case '\t':
			buf.WriteString("\\t")
			i++
		default:
			if b < 0x20 {
				// Control characters need \uXXXX escaping
				buf.WriteString(fmt.Sprintf("\\u%04x", b))
				i++
			} else if b < 0x80 {
				// Regular ASCII
				buf.WriteByte(b)
				i++
			} else {
				// UTF-8 sequence - decode it
				// Note: Surrogates (U+D800-U+DFFF) are encoded as UTF-8 in JS strings
				// even though they're technically invalid UTF-8. We need to detect them
				// and handle them specially.
				r, size := utf8.DecodeRuneInString(s[i:])
				if r == utf8.RuneError && size == 1 {
					// Invalid UTF-8 byte - check if it's part of a surrogate sequence
					// UTF-8 encoding of surrogates (U+D800-U+DFFF) uses bytes ED A0-BF 80-BF
					if b == 0xED && i+2 < len(s) {
						b2 := s[i+1]
						b3 := s[i+2]
						// Check if this forms a surrogate code point (U+D800-U+DFFF)
						if b2 >= 0xA0 && b2 <= 0xBF && b3 >= 0x80 && b3 <= 0xBF {
							// Decode the surrogate value
							highSurr := rune(b&0x0F)<<12 | rune(b2&0x3F)<<6 | rune(b3&0x3F)

							// Check if this is a high surrogate (D800-DBFF) followed by a low surrogate
							if highSurr >= 0xD800 && highSurr <= 0xDBFF && i+5 < len(s) {
								// Check for following low surrogate
								if s[i+3] == 0xED {
									b5 := s[i+4]
									b6 := s[i+5]
									if b5 >= 0xB0 && b5 <= 0xBF && b6 >= 0x80 && b6 <= 0xBF {
										// Low surrogate found - decode it
										lowSurr := rune(s[i+3]&0x0F)<<12 | rune(b5&0x3F)<<6 | rune(b6&0x3F)
										if lowSurr >= 0xDC00 && lowSurr <= 0xDFFF {
											// Valid surrogate pair - combine into single character
											codepoint := 0x10000 + ((highSurr - 0xD800) << 10) + (lowSurr - 0xDC00)
											buf.WriteRune(codepoint)
											i += 6
											continue
										}
									}
								}
							}

							// Lone surrogate - escape as \uXXXX
							buf.WriteString(fmt.Sprintf("\\u%04x", highSurr))
							i += 3
							continue
						}
					}
					// Not a surrogate - escape as \u00XX
					buf.WriteString(fmt.Sprintf("\\u%04x", b))
					i++
				} else if r >= 0xD800 && r <= 0xDFFF {
					// Surrogate code point decoded successfully (shouldn't happen, but handle it)
					buf.WriteString(fmt.Sprintf("\\u%04x", r))
					i += size
				} else {
					// Valid Unicode - write as-is (it's valid UTF-8)
					buf.WriteString(s[i : i+size])
					i += size
				}
			}
		}
	}

	buf.WriteByte('"')
	return buf.String()
}

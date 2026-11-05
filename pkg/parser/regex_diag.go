package parser

import (
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/nooga/paserati/pkg/lexer"
)

// checkRegexScannerDiags reports the diagnostics tsc's scanner raises while
// scanning a unicode-mode (u or v flag) regular expression body: a malformed
// or out-of-range \u{...} escape (TS1125, TS1198) and a brace that is neither
// part of a quantifier nor escaped (TS1508). literal is the whole /body/flags
// token text and bodyEnd the index of the closing slash.
func (p *Parser) checkRegexScannerDiags(literal string, bodyEnd int, flags string) {
	if !strings.ContainsAny(flags, "uv") {
		return
	}
	tok := p.curToken
	at := func(i int, code, msg string) {
		col := tok.Column + len(utf16.Encode([]rune(literal[:i])))
		t := &lexer.Token{Line: tok.Line, Column: col, StartPos: tok.StartPos + i, EndPos: tok.StartPos + i + 1}
		p.addErrorWithCode(t, code, msg)
	}
	isHex := func(c byte) bool {
		return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
	}
	inClass := 0
	for i := 1; i < bodyEnd; i++ {
		switch c := literal[i]; c {
		case '\\':
			if i+1 < bodyEnd && literal[i+1] == 'u' && i+2 < bodyEnd && literal[i+2] == '{' {
				j := i + 3
				start := j
				for j < bodyEnd && isHex(literal[j]) {
					j++
				}
				switch {
				case j == start:
					at(j, "TS1125", "Hexadecimal digit expected.")
					if j < bodyEnd && literal[j] == '}' {
						j++
					}
				default:
					if v, err := strconv.ParseUint(literal[start:j], 16, 32); err != nil || v > 0x10FFFF {
						at(start, "TS1198", "An extended Unicode escape value must be between 0x0 and 0x10FFFF inclusive.")
					}
					if j < bodyEnd && literal[j] == '}' {
						j++
					}
				}
				i = j - 1
				continue
			}
			i++ // any other escape: skip the escaped character
		case '[':
			inClass++
		case ']':
			if inClass > 0 {
				inClass--
			}
		case '{':
			if inClass > 0 {
				continue
			}
			// A quantifier {n}, {n,} or {n,m} is fine.
			j := i + 1
			for j < bodyEnd && literal[j] >= '0' && literal[j] <= '9' {
				j++
			}
			if j > i+1 {
				if j < bodyEnd && literal[j] == ',' {
					j++
					for j < bodyEnd && literal[j] >= '0' && literal[j] <= '9' {
						j++
					}
				}
				if j < bodyEnd && literal[j] == '}' {
					i = j
					continue
				}
			}
			at(i, "TS1508", "Unexpected '{'. Did you mean to escape it with backslash?")
		case '}':
			if inClass == 0 {
				at(i, "TS1508", "Unexpected '}'. Did you mean to escape it with backslash?")
			}
		}
	}
}

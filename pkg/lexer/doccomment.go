package lexer

import "strings"

// recordDocComment remembers input[start:end] if it is a JSDoc comment
// (`/** ... */`, but not the empty `/**/`).
func (l *Lexer) recordDocComment(start, end int) {
	raw := l.input[start:end]
	if len(raw) < 5 || !strings.HasPrefix(raw, "/**") {
		return
	}
	if l.docByEnd == nil {
		l.docByEnd = make(map[int]string)
	}
	l.docByEnd[end] = cleanDocComment(raw)
	l.docRanges = append(l.docRanges, [2]int{start, end})
	if l.docStartByEnd == nil {
		l.docStartByEnd = make(map[int]int)
	}
	l.docStartByEnd[end] = start
}

// DocRanges returns the [start, end) byte ranges of every JSDoc comment
// scanned so far, in source order.
func (l *Lexer) DocRanges() [][2]int { return l.docRanges }

// DocBefore returns the JSDoc comment that sits directly before the token
// starting at byte offset pos, with only whitespace in between. It returns ""
// when there is none. The comment text has its delimiters and the leading `*`
// of each line removed; tags such as `@default x` stay in the text, one per
// line.
func (l *Lexer) DocBefore(pos int) string {
	_, end := l.docEndBefore(pos)
	return l.docByEnd[end]
}

// DocRangeBefore is DocBefore's source range [start, end) of the raw comment;
// ok is false when there is no such comment.
func (l *Lexer) DocRangeBefore(pos int) (start, end int, ok bool) {
	var e int
	start, e = l.docEndBefore(pos)
	if _, found := l.docByEnd[e]; !found {
		return 0, 0, false
	}
	return start, e, true
}

func (l *Lexer) docEndBefore(pos int) (start, end int) {
	if len(l.docByEnd) == 0 {
		return 0, -1
	}
	i := pos
	for i > 0 {
		switch l.input[i-1] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			i--
			continue
		}
		break
	}
	return l.docStartByEnd[i], i
}

func cleanDocComment(raw string) string {
	body := strings.TrimSuffix(strings.TrimPrefix(raw, "/**"), "*/")
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "*") {
			ln = strings.TrimPrefix(ln, "*")
			ln = strings.TrimPrefix(ln, " ")
		}
		lines[i] = strings.TrimRight(ln, " \t")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

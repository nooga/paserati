package checker

import (
	"regexp"
	"strings"

	"github.com/nooga/paserati/pkg/errors"
)

// commentDirectiveRegex is tsc's commentDirectiveRegExSingleLine, matched
// against a line whose text is a `//` comment.
var commentDirectiveRegex = regexp.MustCompile(`^///?\s*@(ts-expect-error|ts-ignore)`)

// applyCommentDirectives drops the diagnostics that a `// @ts-ignore` or
// `// @ts-expect-error` comment suppresses and reports each
// `@ts-expect-error` that suppressed nothing (TS2578). As in tsc
// (markPrecedingCommentDirectiveLine), a directive covers the first line
// after it that is neither blank nor a comment.
func (c *Checker) applyCommentDirectives() {
	if c.source == nil || !strings.Contains(c.source.Content, "@ts-") {
		return
	}
	lines := c.source.Lines()
	if hasNoCheckPragma(lines) {
		c.errors = nil
		return
	}
	type directive struct {
		expectError bool
		used        bool
		column      int
		start       int
	}
	directives := map[int]*directive{} // 0-based line
	offset := 0
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if m := commentDirectiveRegex.FindStringSubmatch(trimmed); m != nil {
			indent := len(line) - len(trimmed)
			directives[i] = &directive{expectError: m[1] == "ts-expect-error", column: indent + 1, start: offset + indent}
		}
		offset += len(line) + 1
	}
	if len(directives) == 0 {
		return
	}
	kept := c.errors[:0]
	for _, err := range c.errors {
		pos := err.Pos()
		if pos.Source != nil && pos.Source != c.source || pos.Line <= 0 {
			kept = append(kept, err)
			continue
		}
		suppressed := false
		for line := pos.Line - 2; line >= 0 && line < len(lines); line-- {
			if d := directives[line]; d != nil {
				d.used = true
				suppressed = true
				break
			}
			text := strings.TrimSpace(lines[line])
			if text != "" && !strings.HasPrefix(text, "//") {
				break
			}
		}
		if !suppressed {
			kept = append(kept, err)
		}
	}
	c.errors = kept
	for line := range lines {
		if d := directives[line]; d != nil && d.expectError && !d.used {
			c.errors = append(c.errors, &errors.TypeError{
				Position: errors.Position{
					Line:     line + 1,
					Column:   d.column,
					StartPos: d.start,
					EndPos:   d.start + 2,
					Source:   c.source,
				},
				Msg:       "Unused '@ts-expect-error' directive.",
				ErrorCode: "TS2578",
			})
		}
	}
}

var noCheckPragmaRegex = regexp.MustCompile(`^///?\s*@ts-nocheck\b`)

// hasNoCheckPragma reports a `// @ts-nocheck` among the file's leading
// comments, which turns off its bind and check diagnostics.
func hasNoCheckPragma(lines []string) bool {
	inBlock := false
	for _, line := range lines {
		text := strings.TrimSpace(line)
		switch {
		case inBlock:
			if strings.Contains(text, "*/") {
				inBlock = false
			}
		case text == "":
		case strings.HasPrefix(text, "//"):
			if noCheckPragmaRegex.MatchString(text) {
				return true
			}
		case strings.HasPrefix(text, "/*"):
			inBlock = !strings.Contains(text[2:], "*/")
		default:
			return false
		}
	}
	return false
}

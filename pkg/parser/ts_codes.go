package parser

import (
	"regexp"

	"github.com/nooga/paserati/pkg/errors"
)

// tsMessageRule maps a parser diagnostic message that is worded exactly like a
// TypeScript diagnostic to that diagnostic's code. Only messages whose text is
// TypeScript's own wording (or a fixed paraphrase of one specific tsc rule)
// belong here; a message that merely resembles a tsc one must be reworded at
// its call site instead of being mapped.
type tsMessageRule struct {
	re   *regexp.Regexp
	code string
	// msg, when non-empty, replaces the message (TypeScript's exact wording).
	msg string
}

var tsMessageRules = []tsMessageRule{
	{re: regexp.MustCompile(`^'[^']+' expected\.$`), code: errors.TS1005},
	{re: regexp.MustCompile(`^'[^']+' or '[^']+' expected\.$`), code: errors.TS1005},
	{re: regexp.MustCompile(`^Expression expected\.$`), code: errors.TS1109},
	{re: regexp.MustCompile(`^Identifier expected\.$`), code: errors.TS1003},
	{re: regexp.MustCompile(`^Declaration or statement expected\.$`), code: errors.TS1128},
	{re: regexp.MustCompile(`^Accessibility modifier already seen\.$`), code: "TS1028"},
	{re: regexp.MustCompile(`^'[a-z]+' modifier must precede '[a-z]+' modifier\.$`), code: "TS1029"},
	{re: regexp.MustCompile(`^Line terminator not permitted before arrow$`), code: "TS1200", msg: "Line terminator not permitted before arrow."},
	{re: regexp.MustCompile(`^An? 'declare' modifier cannot be used in an already ambient context\.$`), code: "TS1038"},
	{re: regexp.MustCompile(`^'[a-z]+' modifier cannot appear on a type member\.$`), code: "TS1070"},
	{re: regexp.MustCompile(`^'[a-z]+' modifier cannot appear on an index signature\.$`), code: "TS1071"},
	{re: regexp.MustCompile(`^'[a-z]+' modifier cannot appear on class elements of this kind\.$`), code: "TS1031"},
	{re: regexp.MustCompile(`^An implementation cannot be declared in ambient contexts\.$`), code: "TS1183"},
	{re: regexp.MustCompile(`^Unexpected token\. A constructor, method, accessor, or property was expected\.$`), code: "TS1068"},
	{re: regexp.MustCompile(`^Unexpected keyword or identifier\.$`), code: "TS1434"},
}

// grammarCodes are diagnostics tsc reports from the checker's grammarError*
// helpers; they are suppressed when the file has any parse diagnostic.
var grammarCodes = map[string]bool{
	"TS1028": true, "TS1029": true, "TS1031": true, "TS1038": true, "TS1049": true,
	"TS1054": true, "TS1070": true, "TS1071": true, "TS1089": true, "TS1108": true,
	"TS1155": true, "TS1163": true, "TS1183": true, "TS1200": true, "TS1206": true,
	"TS18041": true, "TS1042": true, "TS1341": true, "TS1368": true, "TS1044": true, "TS1184": true, "TS1104": true, "TS1105": true, "TS1107": true,
	"TS1115": true, "TS1116": true, "TS1123": true, "TS1182": true,
}

// tsCodeForMessage returns the TypeScript code (and, if different, wording)
// for a parser message, or "" when the message has no mapping.
func tsCodeForMessage(msg string) (code, newMsg string) {
	for i := range tsMessageRules {
		if tsMessageRules[i].re.MatchString(msg) {
			r := &tsMessageRules[i]
			if r.msg != "" {
				return r.code, r.msg
			}
			return r.code, msg
		}
	}
	return "", msg
}

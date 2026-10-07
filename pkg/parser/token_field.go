package parser

import (
	"reflect"

	"github.com/nooga/paserati/pkg/lexer"
)

var lexerTokenPtrType = reflect.TypeOf((*lexer.Token)(nil))

// tokenFieldOf returns the node's `Token *lexer.Token` field when it has one.
// GetTokenFromNode uses it for node types it has no explicit case for, so
// diagnostics reported on them still get a real source position.
func tokenFieldOf(node Node) *lexer.Token {
	if node == nil {
		return nil
	}
	v := reflect.ValueOf(node)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return nil
	}
	v = v.Elem()
	if v.Kind() != reflect.Struct {
		return nil
	}
	f := v.FieldByName("Token")
	if !f.IsValid() || f.Type() != lexerTokenPtrType || f.IsNil() {
		return nil
	}
	return f.Interface().(*lexer.Token)
}

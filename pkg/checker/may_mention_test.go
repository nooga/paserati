package checker

import (
	"testing"

	"github.com/nooga/paserati/pkg/types"
)

// A forward reference with an argument that did not resolve is kept as it is
// by substitution, so it mentions no name: counting it made every object
// reaching fastify's FastifyInstance a copy per substitution.
func TestMayMentionForwardReferenceWithUnresolvedArgument(t *testing.T) {
	names := map[string]types.Type{"TRequest": types.Any}
	param := &types.TypeParameterType{Parameter: &types.TypeParameter{Name: "TRequest"}}
	mention := func(ref types.Type) bool {
		return mayMentionTypeParamNames(ref, names, map[types.Type]bool{}, map[string]map[types.Type]bool{})
	}

	unresolved := &types.GenericTypeAliasForwardReference{AliasName: "AddContentTypeParser", TypeArguments: []types.Type{types.String, nil}}
	if mention(unresolved) {
		t.Error("a reference with an unresolved argument mentions TRequest")
	}
	if got := (&Checker{}).substituteTypes(unresolved, names); got != unresolved {
		t.Error("substitution changed a reference with an unresolved argument")
	}

	resolved := &types.GenericTypeAliasForwardReference{AliasName: "AddContentTypeParser", TypeArguments: []types.Type{types.String, param}}
	if !mention(resolved) {
		t.Error("a reference with TRequest as an argument does not mention it")
	}
}

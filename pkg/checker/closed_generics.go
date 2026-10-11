package checker

import "github.com/nooga/paserati/pkg/types"

// An instantiation of a generic (Set<T>, ZodType<Output>) names in its
// members only the generic's own parameters, already replaced by its
// arguments, when the generic is closed: substituting into it can change
// only the arguments. One whose arguments do not change is kept as it is,
// and a copy with substituted arguments is still an instantiation of the
// generic (assignability relates two by their arguments).

// instantiationUnchanged reports whether substitution leaves typ, an
// instantiation of a closed generic, as it is.
func (c *Checker) instantiationUnchanged(typ *types.ObjectType, substitution map[string]types.Type, visited map[types.Type]types.Type) bool {
	if !c.closedInstantiation(typ, substitution) {
		return false
	}
	for _, a := range typ.GenericArgs {
		if c.substituteTypesWithVisited(a, substitution, visited) != a {
			return false
		}
	}
	return true
}

// closedInstantiation reports whether typ instantiates a closed generic and
// substitution does not rebind the polymorphic this (bound in the members,
// not the arguments).
func (c *Checker) closedInstantiation(typ *types.ObjectType, substitution map[string]types.Type) bool {
	g := typ.Instantiates
	if g == nil || typ.Pending || len(typ.GenericArgs) != len(g.TypeParameters) {
		return false
	}
	if _, rebind := substitution["this"]; rebind {
		return false
	}
	return c.genericClosed(g)
}

// genericClosed reports whether g's body mentions no type parameter but its
// own and those its body declares (signature, mapped and infer parameters):
// a generic declared inside a generic function may name the function's.
func (c *Checker) genericClosed(g *types.GenericType) bool {
	if !c.instCache.complete[g] {
		return false
	}
	if closed, ok := c.instCache.closed[g]; ok {
		return closed
	}
	w := closedWalk{
		params:  map[*types.TypeParameter]bool{},
		visited: map[types.Type]bool{},
	}
	for _, tp := range g.TypeParameters {
		w.params[tp] = true
	}
	closed := !w.foreign(g.Body, nil)
	c.instCache.closed[g] = closed
	return closed
}

type closedWalk struct {
	params  map[*types.TypeParameter]bool
	visited map[types.Type]bool
}

// foreign reports whether t may mention a type parameter not bound inside
// the generic. names are the mapped and infer parameters in scope, which
// are bound by name. Kinds it does not know count as foreign.
func (w *closedWalk) foreign(t types.Type, names []string) bool {
	switch tt := t.(type) {
	case nil, *types.Primitive, *types.LiteralType, *types.ForwardReferenceType, *types.TypeofType:
		return false
	case *types.TypeParameterType:
		if tt.Parameter == nil {
			return true
		}
		if w.params[tt.Parameter] {
			return false
		}
		for _, n := range names {
			if n == tt.Parameter.Name {
				return false
			}
		}
		return true
	case *types.ArrayType:
		return w.foreign(tt.ElementType, names)
	case *types.ReadonlyType:
		return w.foreign(tt.InnerType, names)
	case *types.UnionType:
		return w.any(tt.Types, names)
	case *types.IntersectionType:
		return w.any(tt.Types, names)
	case *types.TupleType:
		return w.any(tt.ElementTypes, names) || w.foreign(tt.RestElementType, names)
	case *types.KeyofType:
		return w.foreign(tt.OperandType, names)
	case *types.IndexedAccessType:
		return w.foreign(tt.ObjectType, names) || w.foreign(tt.IndexType, names)
	case *types.MappedType:
		inner := append(names[:len(names):len(names)], tt.TypeParameter)
		return w.foreign(tt.ConstraintType, names) || w.foreign(tt.ValueType, inner)
	case *types.ConditionalType:
		inner := names[:len(names):len(names)]
		collectInferTypeNames(tt.ExtendsType, &inner, map[types.Type]bool{})
		return w.foreign(tt.CheckType, names) || w.foreign(tt.ExtendsType, inner) ||
			w.foreign(tt.TrueType, inner) || w.foreign(tt.FalseType, names)
	case *types.InferType:
		return false
	case *types.TemplateLiteralType:
		for _, p := range tt.Parts {
			if w.foreign(p.Type, names) {
				return true
			}
		}
		return false
	case *types.InstantiatedType:
		return w.any(tt.TypeArguments, names)
	case *types.ParameterizedForwardReferenceType:
		return w.any(tt.TypeArguments, names)
	case *types.GenericTypeAliasForwardReference:
		return w.any(tt.TypeArguments, names)
	case *types.GenericType:
		return false
	case *types.ObjectType:
		if w.visited[t] {
			return false
		}
		w.visited[t] = true
		for _, p := range tt.Props() {
			if w.foreign(p, names) {
				return true
			}
		}
		if w.any(tt.GenericArgs, names) || w.any(tt.Bases(), names) {
			return true
		}
		for _, sigs := range [][]*types.Signature{tt.CallSigs(), tt.ConstructSigs()} {
			for _, sig := range sigs {
				if sig == nil {
					continue
				}
				for _, tp := range sig.TypeParameters {
					w.params[tp] = true
				}
				for _, tp := range sig.TypeParameters {
					if w.foreign(tp.Constraint, names) || w.foreign(tp.Default, names) {
						return true
					}
				}
				if w.any(sig.ParameterTypes, names) || w.foreign(sig.ReturnType, names) ||
					w.foreign(sig.RestParameterType, names) || w.foreign(sig.ThisType, names) {
					return true
				}
			}
		}
		for _, sig := range tt.IndexSigs() {
			if sig == nil {
				continue
			}
			inner := names
			if sig.IsMapped && sig.TypeParameter != "" {
				inner = append(names[:len(names):len(names)], sig.TypeParameter)
			}
			if w.foreign(sig.KeyType, inner) || w.foreign(sig.ValueType, inner) || w.foreign(sig.ConstraintType, names) {
				return true
			}
		}
		return false
	}
	return true
}

func (w *closedWalk) any(ts []types.Type, names []string) bool {
	for _, t := range ts {
		if w.foreign(t, names) {
			return true
		}
	}
	return false
}

// collectInferTypeNames appends the names `infer X` declares in t.
func collectInferTypeNames(t types.Type, names *[]string, seen map[types.Type]bool) {
	switch tt := t.(type) {
	case *types.InferType:
		*names = append(*names, tt.TypeParameter)
	case *types.ArrayType:
		collectInferTypeNames(tt.ElementType, names, seen)
	case *types.ReadonlyType:
		collectInferTypeNames(tt.InnerType, names, seen)
	case *types.UnionType:
		for _, m := range tt.Types {
			collectInferTypeNames(m, names, seen)
		}
	case *types.IntersectionType:
		for _, m := range tt.Types {
			collectInferTypeNames(m, names, seen)
		}
	case *types.TupleType:
		for _, m := range tt.ElementTypes {
			collectInferTypeNames(m, names, seen)
		}
		collectInferTypeNames(tt.RestElementType, names, seen)
	case *types.TemplateLiteralType:
		for _, p := range tt.Parts {
			collectInferTypeNames(p.Type, names, seen)
		}
	case *types.InstantiatedType:
		for _, a := range tt.TypeArguments {
			collectInferTypeNames(a, names, seen)
		}
	case *types.ObjectType:
		if seen[t] {
			return
		}
		seen[t] = true
		for _, p := range tt.Props() {
			collectInferTypeNames(p, names, seen)
		}
		for _, a := range tt.GenericArgs {
			collectInferTypeNames(a, names, seen)
		}
		for _, sigs := range [][]*types.Signature{tt.CallSigs(), tt.ConstructSigs()} {
			for _, sig := range sigs {
				if sig == nil {
					continue
				}
				for _, p := range sig.ParameterTypes {
					collectInferTypeNames(p, names, seen)
				}
				collectInferTypeNames(sig.ReturnType, names, seen)
				collectInferTypeNames(sig.RestParameterType, names, seen)
			}
		}
	}
}

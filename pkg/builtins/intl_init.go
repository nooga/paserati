package builtins

import (
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// PriorityIntl: Intl only needs Object, Symbol and %Iterator.prototype%,
// all of which are in place well before the Date/Temporal tier.
const PriorityIntl = PriorityDate + 5

// IntlInitializer installs the ECMA-402 Intl namespace object: Intl.Segmenter
// (#210), Intl.NumberFormat, DateTimeFormat, Collator, PluralRules and
// ListFormat (#624), the namespace's own
// Intl.getCanonicalLocales and @@toStringTag. Constructors that are not
// implemented are absent rather than stubbed, so feature detection
// (`"DisplayNames" in Intl`) stays truthful.
type IntlInitializer struct{}

func (i *IntlInitializer) Name() string  { return "Intl" }
func (i *IntlInitializer) Priority() int { return PriorityIntl }

func (i *IntlInitializer) InitTypes(ctx *TypeContext) error {
	stringArray := &types.ArrayType{ElementType: types.String}

	// interface Intl.SegmentData { segment: string; index: number; input: string; isWordLike?: boolean }
	segmentDataType := types.NewObjectType().
		WithProperty("segment", types.String).
		WithProperty("index", types.Number).
		WithProperty("input", types.String).
		WithOptionalProperty("isWordLike", types.Boolean)

	segmentIteratorResultType := types.NewObjectType().
		WithProperty("value", segmentDataType).
		WithProperty("done", types.Boolean)
	segmentIteratorType := types.NewObjectType().
		WithProperty("next", types.NewSimpleFunction([]types.Type{}, segmentIteratorResultType))
	// [Symbol.iterator] on the iterator itself, so it is iterable too.
	segmentIteratorType.WithProperty("__COMPUTED_PROPERTY__", types.NewSimpleFunction([]types.Type{}, segmentIteratorType))

	// interface Intl.Segments { containing(index?: number): SegmentData | undefined; [Symbol.iterator](): Iterator<SegmentData> }
	segmentsType := types.NewObjectType().
		WithProperty("containing", types.NewOptionalFunction([]types.Type{types.Number},
			types.NewUnionType(segmentDataType, types.Undefined), []bool{true})).
		WithProperty("__COMPUTED_PROPERTY__", types.NewSimpleFunction([]types.Type{}, segmentIteratorType))

	resolvedOptionsType := types.NewObjectType().
		WithProperty("locale", types.String).
		WithProperty("granularity", types.String)

	// interface Intl.Segmenter { segment(input: string): Segments; resolvedOptions(): ResolvedSegmenterOptions }
	segmenterType := types.NewObjectType().
		WithProperty("segment", types.NewSimpleFunction([]types.Type{types.String}, segmentsType)).
		WithProperty("resolvedOptions", types.NewSimpleFunction([]types.Type{}, resolvedOptionsType))

	// new Intl.Segmenter(locales?: string | string[], options?: { localeMatcher?, granularity? })
	segmenterCtorType := types.NewObjectType().
		WithConstructSignature(&types.Signature{
			ParameterTypes: []types.Type{types.Any, types.Any},
			ReturnType:     segmenterType,
			OptionalParams: []bool{true, true},
		}).
		WithProperty("supportedLocalesOf", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, stringArray, []bool{false, true})).
		WithProperty("prototype", segmenterType)

	// interface Intl.NumberFormatPart { type: string; value: string }
	numberPartType := types.NewObjectType().
		WithProperty("type", types.String).
		WithProperty("value", types.String)
	numberRangePartType := types.NewObjectType().
		WithProperty("type", types.String).
		WithProperty("value", types.String).
		WithProperty("source", types.String)
	numberFormatType := types.NewObjectType().
		WithProperty("format", types.NewSimpleFunction([]types.Type{types.Any}, types.String)).
		WithProperty("formatToParts", types.NewOptionalFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: numberPartType}, []bool{true})).
		WithProperty("formatRange", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.String)).
		WithProperty("formatRangeToParts", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, &types.ArrayType{ElementType: numberRangePartType})).
		WithProperty("resolvedOptions", types.NewSimpleFunction([]types.Type{}, types.Any))
	numberFormatCtorType := types.NewObjectType().
		WithConstructSignature(&types.Signature{
			ParameterTypes: []types.Type{types.Any, types.Any},
			ReturnType:     numberFormatType,
			OptionalParams: []bool{true, true},
		}).
		WithCallSignature(&types.Signature{
			ParameterTypes: []types.Type{types.Any, types.Any},
			ReturnType:     numberFormatType,
			OptionalParams: []bool{true, true},
		}).
		WithProperty("supportedLocalesOf", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, stringArray, []bool{false, true})).
		WithProperty("prototype", numberFormatType)

	dateTimeFormatType := types.NewObjectType().
		WithProperty("format", types.NewOptionalFunction([]types.Type{types.Any}, types.String, []bool{true})).
		WithProperty("formatToParts", types.NewOptionalFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: numberPartType}, []bool{true})).
		WithProperty("formatRange", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.String)).
		WithProperty("formatRangeToParts", types.NewSimpleFunction([]types.Type{types.Any, types.Any}, &types.ArrayType{ElementType: numberRangePartType})).
		WithProperty("resolvedOptions", types.NewSimpleFunction([]types.Type{}, types.Any))
	dateTimeFormatCtorType := types.NewObjectType().
		WithConstructSignature(&types.Signature{
			ParameterTypes: []types.Type{types.Any, types.Any},
			ReturnType:     dateTimeFormatType,
			OptionalParams: []bool{true, true},
		}).
		WithCallSignature(&types.Signature{
			ParameterTypes: []types.Type{types.Any, types.Any},
			ReturnType:     dateTimeFormatType,
			OptionalParams: []bool{true, true},
		}).
		WithProperty("supportedLocalesOf", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, stringArray, []bool{false, true})).
		WithProperty("prototype", dateTimeFormatType)

	ctorOf := func(instance types.Type, callable bool) *types.ObjectType {
		sig := &types.Signature{ParameterTypes: []types.Type{types.Any, types.Any}, ReturnType: instance, OptionalParams: []bool{true, true}}
		ctor := types.NewObjectType().WithConstructSignature(sig).
			WithProperty("supportedLocalesOf", types.NewOptionalFunction([]types.Type{types.Any, types.Any}, stringArray, []bool{false, true})).
			WithProperty("prototype", instance)
		if callable {
			ctor.WithCallSignature(sig)
		}
		return ctor
	}
	collatorType := types.NewObjectType().
		WithProperty("compare", types.NewSimpleFunction([]types.Type{types.String, types.String}, types.Number)).
		WithProperty("resolvedOptions", types.NewSimpleFunction([]types.Type{}, types.Any))
	pluralRulesType := types.NewObjectType().
		WithProperty("select", types.NewSimpleFunction([]types.Type{types.Number}, types.String)).
		WithProperty("selectRange", types.NewSimpleFunction([]types.Type{types.Number, types.Number}, types.String)).
		WithProperty("resolvedOptions", types.NewSimpleFunction([]types.Type{}, types.Any))
	listFormatType := types.NewObjectType().
		WithProperty("format", types.NewOptionalFunction([]types.Type{types.Any}, types.String, []bool{true})).
		WithProperty("formatToParts", types.NewOptionalFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: numberPartType}, []bool{true})).
		WithProperty("resolvedOptions", types.NewSimpleFunction([]types.Type{}, types.Any))

	intlNamespace := types.NewNamespaceType("Intl")
	intlNamespace.ValueShape.
		WithProperty("NumberFormat", numberFormatCtorType).
		WithProperty("DateTimeFormat", dateTimeFormatCtorType).
		WithProperty("Collator", ctorOf(collatorType, true)).
		WithProperty("PluralRules", ctorOf(pluralRulesType, false)).
		WithProperty("ListFormat", ctorOf(listFormatType, false)).
		WithProperty("Segmenter", segmenterCtorType).
		WithProperty("getCanonicalLocales", types.NewOptionalFunction([]types.Type{types.Any}, stringArray, []bool{true}))
	intlNamespace.TypeMembers["NumberFormat"] = numberFormatType
	intlNamespace.TypeMembers["DateTimeFormat"] = dateTimeFormatType
	intlNamespace.TypeMembers["Collator"] = collatorType
	intlNamespace.TypeMembers["PluralRules"] = pluralRulesType
	intlNamespace.TypeMembers["ListFormat"] = listFormatType
	intlNamespace.TypeMembers["DateTimeFormatPart"] = numberPartType
	intlNamespace.TypeMembers["NumberFormatPart"] = numberPartType
	intlNamespace.TypeMembers["Segmenter"] = segmenterType
	intlNamespace.TypeMembers["Segments"] = segmentsType
	intlNamespace.TypeMembers["SegmentData"] = segmentDataType
	intlNamespace.TypeMembers["ResolvedSegmenterOptions"] = resolvedOptionsType

	// The value binding is the namespace object's shape (so `Intl.Segmenter`
	// resolves as an ordinary property); the type binding is the namespace
	// itself (so `Intl.Segmenter` also resolves in type position).
	if err := ctx.DefineGlobal("Intl", intlNamespace.ValueShape); err != nil {
		return err
	}
	return ctx.DefineTypeAlias("Intl", intlNamespace)
}

func (i *IntlInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	intlObj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	intlDefineToStringTag(vmInstance, intlObj, "Intl")

	// Intl.getCanonicalLocales(locales)
	intlObj.SetOwnNonEnumerable("getCanonicalLocales", vm.NewNativeFunction(1, false, "getCanonicalLocales", func(args []vm.Value) (vm.Value, error) {
		list, err := intlCanonicalizeLocaleList(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		return intlStringList(vmInstance, list), nil
	}))

	installIntlSegmenter(vmInstance, intlObj)
	installIntlNumberFormat(vmInstance, intlObj)
	installIntlDateTimeFormat(vmInstance, intlObj)
	installIntlCollator(vmInstance, intlObj)
	installIntlPluralRules(vmInstance, intlObj)
	installIntlListFormat(vmInstance, intlObj)

	return ctx.DefineGlobal("Intl", vm.NewValueFromPlainObject(intlObj))
}

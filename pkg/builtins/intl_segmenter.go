package builtins

import (
	"math"
	"sort"
	"unicode"

	"github.com/rivo/uniseg"

	"github.com/nooga/paserati/pkg/vm"
)

// Intl.Segmenter (ECMA-402 §18) over UAX #29 default text segmentation, via
// rivo/uniseg. This is what #210's real consumer (pi-tui's terminal string
// measuring/wrapping) needs: default-locale grapheme and word boundaries,
// walked through segment(str) / for-of / containing(index).
//
// Segmentation is locale-independent here - UAX #29's default rules with no
// locale tailoring and no dictionary-based word breaking (Thai, Lao, Khmer
// and CJK text therefore break per character for "word", where ICU would
// consult a dictionary). isWordLike is "the segment contains a letter or a
// number", the observable effect of ICU's rule-status classes.
//
// Strings are canonical WTF-8 in the VM (see pkg/wtf8: a surrogate pair is
// always one 4-byte sequence), but a lone surrogate is a 3-byte sequence
// uniseg would split byte-by-byte. So the input is normalized once more as a
// cheap safety net (a no-op for canonical strings), then each lone surrogate
// is swapped for a same-width stand-in with the surrogate's break properties
// (see intlSanitizeWTF8). Byte offsets from uniseg therefore map 1:1 onto the
// normalized text, which is what segments are sliced from.

type intlGranularity uint8

const (
	intlGranularityGrapheme intlGranularity = iota
	intlGranularityWord
	intlGranularitySentence
)

func (g intlGranularity) String() string {
	switch g {
	case intlGranularityWord:
		return "word"
	case intlGranularitySentence:
		return "sentence"
	default:
		return "grapheme"
	}
}

var intlGranularityNames = []string{"grapheme", "word", "sentence"}

// intlSegmenter is an Intl.Segmenter instance's internal slots
// ([[Locale]], [[SegmenterGranularity]]).
type intlSegmenter struct {
	locale      string
	granularity intlGranularity
}

// intlSegmentRun is one segment: byte offsets into the original string plus
// its UTF-16 start index (what JS observes) and, for "word", word-likeness.
type intlSegmentRun struct {
	start, end int
	u16Start   int
	wordLike   bool
}

// intlSegments is a Segments instance's internal slots ([[SegmentsSegmenter]],
// [[SegmentsString]]). The string is immutable, so its full segmentation is
// computed once, lazily, and shared by every iterator and containing() call -
// equivalent to the spec's FindBoundary-from-the-start on every step, minus
// the quadratic cost.
type intlSegments struct {
	segmenter *intlSegmenter
	input     string // as passed to segment(): the `input` property of every segment
	text      string // input with WTF-8 surrogate pairs joined; runs index into this
	runs      []intlSegmentRun
	computed  bool
}

// intlSegmentIterator is a Segment Iterator's internal slots
// ([[IteratingSegmenter]], [[IteratedString]], [[IteratedStringNextSegmentCodeUnitIndex]]).
type intlSegmentIterator struct {
	segments *intlSegments
	pos      int
}

// Same-width stand-ins for WTF-8 lone surrogates (both 3 bytes, like the
// surrogate encoding). Surrogates have Grapheme_Cluster_Break=Control (so a
// following mark does not attach), and Word_Break / Sentence_Break = Other.
const (
	intlSurrogateStandInGrapheme = "​" // ZERO WIDTH SPACE: GCB=Control
	intlSurrogateStandInOther    = "�" // REPLACEMENT CHARACTER: WB=Other, SB=Other
)

// intlHasWTF8Surrogate reports whether s contains a WTF-8 encoded surrogate
// (ED A0..BF xx - a byte pattern valid UTF-8 never produces).
func intlHasWTF8Surrogate(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == 0xED && s[i+1] >= 0xA0 && s[i+1] <= 0xBF {
			return true
		}
	}
	return false
}

// intlNormalizeWTF8 joins adjacent WTF-8 lead+trail surrogates into the
// 4-byte UTF-8 character they denote, leaving lone surrogates as WTF-8. The
// UTF-16 view is unchanged, so the result is the same JS string. Returns s
// itself when it holds no surrogate bytes at all.
func intlNormalizeWTF8(s string) string {
	if !intlHasWTF8Surrogate(s) {
		return s
	}
	return vm.UTF16ToString(vm.StringToUTF16(s))
}

// intlSanitizeWTF8 replaces every WTF-8 encoded lone surrogate with
// replacement, which must be exactly three bytes so offsets are preserved.
// Returns s itself when there is nothing to replace.
func intlSanitizeWTF8(s string, replacement string) string {
	var b []byte
	for i := 0; i+2 < len(s); i++ {
		if s[i] == 0xED && s[i+1] >= 0xA0 && s[i+1] <= 0xBF && s[i+2] >= 0x80 && s[i+2] <= 0xBF {
			if b == nil {
				b = []byte(s)
			}
			copy(b[i:i+3], replacement)
			i += 2
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

func intlIsWordLike(segment string) bool {
	for _, r := range segment {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func (s *intlSegments) ensureRuns() {
	if s.computed {
		return
	}
	s.computed = true
	granularity := s.segmenter.granularity
	standIn := intlSurrogateStandInOther
	if granularity == intlGranularityGrapheme {
		standIn = intlSurrogateStandInGrapheme
	}
	s.text = intlNormalizeWTF8(s.input)
	text := intlSanitizeWTF8(s.text, standIn)

	state := -1
	offset := 0
	rest := text
	for len(rest) > 0 {
		var segment string
		switch granularity {
		case intlGranularityWord:
			segment, rest, state = uniseg.FirstWordInString(rest, state)
		case intlGranularitySentence:
			segment, rest, state = uniseg.FirstSentenceInString(rest, state)
		default:
			segment, rest, _, state = uniseg.FirstGraphemeClusterInString(rest, state)
		}
		if len(segment) == 0 { // defensive: never spin on a zero-width step
			break
		}
		end := offset + len(segment)
		run := intlSegmentRun{start: offset, end: end, u16Start: vm.ByteToUTF16Offset(s.text, offset)}
		if granularity == intlGranularityWord {
			run.wordLike = intlIsWordLike(s.text[offset:end])
		}
		s.runs = append(s.runs, run)
		offset = end
	}
}

// dataObject implements CreateSegmentDataObject for run i: a fresh ordinary
// object with segment, index, input and - for word granularity - isWordLike,
// in that order.
func (s *intlSegments) dataObject(vmInstance *vm.VM, i int) vm.Value {
	run := s.runs[i]
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	obj.SetOwn("segment", vm.NewString(s.text[run.start:run.end]))
	obj.SetOwn("index", vm.NumberValue(float64(run.u16Start)))
	obj.SetOwn("input", vm.NewString(s.input))
	if s.segmenter.granularity == intlGranularityWord {
		obj.SetOwn("isWordLike", vm.BooleanValue(run.wordLike))
	}
	return vm.NewValueFromPlainObject(obj)
}

// containing implements the core of %SegmentsPrototype%.containing: the
// segment whose UTF-16 span covers code unit index n, or undefined when n is
// out of range.
func (s *intlSegments) containing(vmInstance *vm.VM, n float64) vm.Value {
	if n < 0 || n >= float64(vm.UTF16Length(s.input)) {
		return vm.Undefined
	}
	s.ensureRuns()
	idx := int(n)
	i := sort.Search(len(s.runs), func(i int) bool { return s.runs[i].u16Start > idx }) - 1
	if i < 0 {
		return vm.Undefined
	}
	return s.dataObject(vmInstance, i)
}

func intlSlotsOf(v vm.Value) any {
	if v.Type() != vm.TypeObject {
		return nil
	}
	return v.AsPlainObject().InternalSlots()
}

func intlIterResult(vmInstance *vm.VM, value vm.Value, done bool) vm.Value {
	obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	obj.SetOwn("value", value)
	obj.SetOwn("done", vm.BooleanValue(done))
	return vm.NewValueFromPlainObject(obj)
}

func intlDefineToStringTag(vmInstance *vm.VM, obj *vm.PlainObject, tag string) {
	if vmInstance.SymbolToStringTag.Type() != vm.TypeSymbol {
		return
	}
	writable, enumerable, configurable := false, false, true
	obj.DefineOwnPropertyByKey(vm.NewSymbolKey(vmInstance.SymbolToStringTag), vm.NewString(tag),
		&writable, &enumerable, &configurable)
}

// installIntlSegmenter builds Intl.Segmenter, %Segmenter.prototype%,
// %SegmentsPrototype% and %SegmentIteratorPrototype% and installs the
// constructor on the Intl object.
func installIntlSegmenter(vmInstance *vm.VM, intlObj *vm.PlainObject) {
	segmenterProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	segmenterProtoVal := vm.NewValueFromPlainObject(segmenterProto)
	segmentsProto := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
	segmentsProtoVal := vm.NewValueFromPlainObject(segmentsProto)
	iteratorProto := vm.NewObject(vmInstance.IteratorPrototype).AsPlainObject()
	iteratorProtoVal := vm.NewValueFromPlainObject(iteratorProto)

	// Register the realm intrinsic so GetPrototypeFromConstructor can fall
	// back to the *constructor's* realm's prototype for cross-realm
	// Reflect.construct, like every other builtin prototype.
	vmInstance.IntlSegmenterPrototype = segmenterProtoVal

	// ---- Intl.Segmenter(locales, options) ----
	ctor := vm.NewConstructorWithProps(0, false, "Segmenter", func(args []vm.Value) (vm.Value, error) {
		newTarget := vmInstance.GetNewTarget()
		if newTarget.IsUndefined() {
			return vm.Undefined, vmInstance.NewTypeError("Constructor Intl.Segmenter requires 'new'")
		}
		requested, err := intlCanonicalizeLocaleList(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		options, err := intlGetOptionsObject(vmInstance, intlArg(args, 1))
		if err != nil {
			return intlAbrupt(err)
		}
		// Options are read in spec order: localeMatcher, then (after
		// ResolveLocale) granularity. lineBreakStyle is not an option.
		if _, err := intlGetStringOption(vmInstance, options, "localeMatcher", "Intl.Segmenter", intlLocaleMatchers, "best fit"); err != nil {
			return intlAbrupt(err)
		}
		locale := intlLookupMatcher(requested)
		granularityName, err := intlGetStringOption(vmInstance, options, "granularity", "Intl.Segmenter", intlGranularityNames, "grapheme")
		if err != nil {
			return intlAbrupt(err)
		}
		granularity := intlGranularityGrapheme
		for i, name := range intlGranularityNames {
			if name == granularityName {
				granularity = intlGranularity(i)
			}
		}

		// OrdinaryCreateFromConstructor(NewTarget, "%Intl.Segmenter.prototype%"):
		// a throwing "prototype" getter propagates; a non-object falls back
		// to newTarget's realm's %Intl.Segmenter.prototype%.
		proto, err := vmInstance.GetPrototypeFromConstructor(newTarget, "%Intl.Segmenter.prototype%")
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(proto).AsPlainObject()
		obj.SetInternalSlots(&intlSegmenter{locale: locale, granularity: granularity})
		return vm.NewValueFromPlainObject(obj), nil
	})
	ctorProps := ctor.AsNativeFunctionWithProps().Properties
	ctorProps.DefineFixedProperty("prototype", segmenterProtoVal)
	ctorProps.SetOwnNonEnumerable("supportedLocalesOf", vm.NewNativeFunction(1, false, "supportedLocalesOf", func(args []vm.Value) (vm.Value, error) {
		return intlSupportedLocalesOf(vmInstance, "Intl.Segmenter", intlArg(args, 0), intlArg(args, 1))
	}))

	// ---- %Segmenter.prototype% ----
	segmenterProto.SetOwnNonEnumerable("constructor", ctor)
	intlDefineToStringTag(vmInstance, segmenterProto, "Intl.Segmenter")

	segmenterProto.SetOwnNonEnumerable("segment", vm.NewNativeFunction(1, false, "segment", func(args []vm.Value) (vm.Value, error) {
		segmenter, ok := intlSlotsOf(vmInstance.GetThis()).(*intlSegmenter)
		if !ok {
			return vm.Undefined, vmInstance.NewTypeError("Method Intl.Segmenter.prototype.segment called on incompatible receiver")
		}
		input, err := getStringValueWithVM(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		obj := vm.NewObject(segmentsProtoVal).AsPlainObject()
		obj.SetInternalSlots(&intlSegments{segmenter: segmenter, input: input})
		return vm.NewValueFromPlainObject(obj), nil
	}))

	segmenterProto.SetOwnNonEnumerable("resolvedOptions", vm.NewNativeFunction(0, false, "resolvedOptions", func(args []vm.Value) (vm.Value, error) {
		segmenter, ok := intlSlotsOf(vmInstance.GetThis()).(*intlSegmenter)
		if !ok {
			return vm.Undefined, vmInstance.NewTypeError("Method Intl.Segmenter.prototype.resolvedOptions called on incompatible receiver")
		}
		obj := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()
		obj.SetOwn("locale", vm.NewString(segmenter.locale))
		obj.SetOwn("granularity", vm.NewString(segmenter.granularity.String()))
		return vm.NewValueFromPlainObject(obj), nil
	}))

	// ---- %SegmentsPrototype% ----
	segmentsProto.SetOwnNonEnumerable("containing", vm.NewNativeFunction(1, false, "containing", func(args []vm.Value) (vm.Value, error) {
		segments, ok := intlSlotsOf(vmInstance.GetThis()).(*intlSegments)
		if !ok {
			return vm.Undefined, vmInstance.NewTypeError("Method %Segments.prototype%.containing called on incompatible receiver")
		}
		// ToIntegerOrInfinity(index): Symbols and BigInts throw, objects go
		// through ToPrimitive, NaN is 0, and the fraction is truncated.
		f, err := toNumberWithVM(vmInstance, intlArg(args, 0))
		if err != nil {
			return intlAbrupt(err)
		}
		if math.IsNaN(f) {
			f = 0
		} else if !math.IsInf(f, 0) {
			f = math.Trunc(f)
		}
		return segments.containing(vmInstance, f), nil
	}))

	writable, enumerable, configurable := true, false, true
	segmentsProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator),
		vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
			segments, ok := intlSlotsOf(vmInstance.GetThis()).(*intlSegments)
			if !ok {
				return vm.Undefined, vmInstance.NewTypeError("Method %Segments.prototype%[Symbol.iterator] called on incompatible receiver")
			}
			it := vm.NewObject(iteratorProtoVal).AsPlainObject()
			it.SetInternalSlots(&intlSegmentIterator{segments: segments})
			return vm.NewValueFromPlainObject(it), nil
		}), &writable, &enumerable, &configurable)

	// ---- %SegmentIteratorPrototype% ----
	intlDefineToStringTag(vmInstance, iteratorProto, "Segmenter String Iterator")
	iteratorProto.SetOwnNonEnumerable("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		it, ok := intlSlotsOf(vmInstance.GetThis()).(*intlSegmentIterator)
		if !ok {
			return vm.Undefined, vmInstance.NewTypeError("Method %SegmentIterator.prototype%.next called on incompatible receiver")
		}
		segments := it.segments
		segments.ensureRuns()
		if it.pos >= len(segments.runs) {
			return intlIterResult(vmInstance, vm.Undefined, true), nil
		}
		value := segments.dataObject(vmInstance, it.pos)
		it.pos++
		return intlIterResult(vmInstance, value, false), nil
	}))

	intlObj.SetOwnNonEnumerable("Segmenter", ctor)
}

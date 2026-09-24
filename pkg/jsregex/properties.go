package jsregex

import "sort"

// The property names and values a \p{...} escape may use, per ECMA-262
// §22.2.2.9 (tables "Non-binary Unicode property aliases", "Binary Unicode
// property aliases", "Binary Unicode properties of strings") and the value
// aliases of PropertyValueAliases.txt. Matching is case-sensitive and exact:
// no loose matching, no Is/In prefixes.

// PropertyKind says which family a resolved property escape belongs to.
type PropertyKind int

const (
	PropertyInvalid PropertyKind = iota
	PropertyGeneralCategory
	PropertyScript
	PropertyScriptExtensions
	PropertyBinary
	PropertyOfStrings
)

// ResolveProperty resolves the body of a \p{...} escape (value is "" for
// the lone form) to its family and canonical name: the long property name
// for a binary property or property of strings, the long value name for a
// General_Category, Script or Script_Extensions value.
func ResolveProperty(name, value string, hasValue bool) (PropertyKind, string) {
	if hasValue {
		switch name {
		case "General_Category", "gc":
			if c, ok := generalCategoryValues[value]; ok {
				return PropertyGeneralCategory, c
			}
		case "Script", "sc":
			if c, ok := scriptValues[value]; ok {
				return PropertyScript, c
			}
		case "Script_Extensions", "scx":
			if c, ok := scriptValues[value]; ok {
				return PropertyScriptExtensions, c
			}
		}
		return PropertyInvalid, ""
	}
	if c, ok := generalCategoryValues[name]; ok {
		return PropertyGeneralCategory, c
	}
	if c, ok := binaryProperties[name]; ok {
		return PropertyBinary, c
	}
	if stringProperties[name] {
		return PropertyOfStrings, name
	}
	return PropertyInvalid, ""
}

// IsValidPropertyValue reports whether name=value is a valid
// UnicodePropertyName=UnicodePropertyValue pair.
func IsValidPropertyValue(name, value string) bool {
	k, _ := ResolveProperty(name, value, true)
	return k != PropertyInvalid
}

// IsValidLoneProperty reports whether name is a valid
// LoneUnicodePropertyNameOrValue that is not a property of strings.
func IsValidLoneProperty(name string) bool {
	k, _ := ResolveProperty(name, "", false)
	return k != PropertyInvalid && k != PropertyOfStrings
}

// IsPropertyOfStrings reports whether name is a binary property of strings,
// valid only in UnicodeSets mode.
func IsPropertyOfStrings(name string) bool { return stringProperties[name] }

func register(m map[string]string, canonical string, aliases ...string) {
	m[canonical] = canonical
	for _, a := range aliases {
		m[a] = canonical
	}
}

var generalCategoryValues = map[string]string{}
var binaryProperties = map[string]string{}
var scriptValues = map[string]string{}

var stringProperties = map[string]bool{
	"Basic_Emoji":                 true,
	"Emoji_Keycap_Sequence":       true,
	"RGI_Emoji_Modifier_Sequence": true,
	"RGI_Emoji_Flag_Sequence":     true,
	"RGI_Emoji_Tag_Sequence":      true,
	"RGI_Emoji_ZWJ_Sequence":      true,
	"RGI_Emoji":                   true,
}

func init() {
	gc := generalCategoryValues
	register(gc, "Other", "C")
	register(gc, "Control", "Cc", "cntrl")
	register(gc, "Format", "Cf")
	register(gc, "Unassigned", "Cn")
	register(gc, "Private_Use", "Co")
	register(gc, "Surrogate", "Cs")
	register(gc, "Letter", "L")
	register(gc, "Cased_Letter", "LC")
	register(gc, "Lowercase_Letter", "Ll")
	register(gc, "Modifier_Letter", "Lm")
	register(gc, "Other_Letter", "Lo")
	register(gc, "Titlecase_Letter", "Lt")
	register(gc, "Uppercase_Letter", "Lu")
	register(gc, "Mark", "M", "Combining_Mark")
	register(gc, "Spacing_Mark", "Mc")
	register(gc, "Enclosing_Mark", "Me")
	register(gc, "Nonspacing_Mark", "Mn")
	register(gc, "Number", "N")
	register(gc, "Decimal_Number", "Nd", "digit")
	register(gc, "Letter_Number", "Nl")
	register(gc, "Other_Number", "No")
	register(gc, "Punctuation", "P", "punct")
	register(gc, "Connector_Punctuation", "Pc")
	register(gc, "Dash_Punctuation", "Pd")
	register(gc, "Close_Punctuation", "Pe")
	register(gc, "Final_Punctuation", "Pf")
	register(gc, "Initial_Punctuation", "Pi")
	register(gc, "Other_Punctuation", "Po")
	register(gc, "Open_Punctuation", "Ps")
	register(gc, "Symbol", "S")
	register(gc, "Currency_Symbol", "Sc")
	register(gc, "Modifier_Symbol", "Sk")
	register(gc, "Math_Symbol", "Sm")
	register(gc, "Other_Symbol", "So")
	register(gc, "Separator", "Z")
	register(gc, "Line_Separator", "Zl")
	register(gc, "Paragraph_Separator", "Zp")
	register(gc, "Space_Separator", "Zs")

	b := binaryProperties
	register(b, "ASCII")
	register(b, "ASCII_Hex_Digit", "AHex")
	register(b, "Alphabetic", "Alpha")
	register(b, "Any")
	register(b, "Assigned")
	register(b, "Bidi_Control", "Bidi_C")
	register(b, "Bidi_Mirrored", "Bidi_M")
	register(b, "Case_Ignorable", "CI")
	register(b, "Cased")
	register(b, "Changes_When_Casefolded", "CWCF")
	register(b, "Changes_When_Casemapped", "CWCM")
	register(b, "Changes_When_Lowercased", "CWL")
	register(b, "Changes_When_NFKC_Casefolded", "CWKCF")
	register(b, "Changes_When_Titlecased", "CWT")
	register(b, "Changes_When_Uppercased", "CWU")
	register(b, "Dash")
	register(b, "Default_Ignorable_Code_Point", "DI")
	register(b, "Deprecated", "Dep")
	register(b, "Diacritic", "Dia")
	register(b, "Emoji")
	register(b, "Emoji_Component", "EComp")
	register(b, "Emoji_Modifier", "EMod")
	register(b, "Emoji_Modifier_Base", "EBase")
	register(b, "Emoji_Presentation", "EPres")
	register(b, "Extended_Pictographic", "ExtPict")
	register(b, "Extender", "Ext")
	register(b, "Grapheme_Base", "Gr_Base")
	register(b, "Grapheme_Extend", "Gr_Ext")
	register(b, "Hex_Digit", "Hex")
	register(b, "IDS_Binary_Operator", "IDSB")
	register(b, "IDS_Trinary_Operator", "IDST")
	register(b, "ID_Continue", "IDC")
	register(b, "ID_Start", "IDS")
	register(b, "Ideographic", "Ideo")
	register(b, "Join_Control", "Join_C")
	register(b, "Logical_Order_Exception", "LOE")
	register(b, "Lowercase", "Lower")
	register(b, "Math")
	register(b, "Noncharacter_Code_Point", "NChar")
	register(b, "Pattern_Syntax", "Pat_Syn")
	register(b, "Pattern_White_Space", "Pat_WS")
	register(b, "Quotation_Mark", "QMark")
	register(b, "Radical")
	register(b, "Regional_Indicator", "RI")
	register(b, "Sentence_Terminal", "STerm")
	register(b, "Soft_Dotted", "SD")
	register(b, "Terminal_Punctuation", "Term")
	register(b, "Unified_Ideograph", "UIdeo")
	register(b, "Uppercase", "Upper")
	register(b, "Variation_Selector", "VS")
	register(b, "White_Space", "space")
	register(b, "XID_Continue", "XIDC")
	register(b, "XID_Start", "XIDS")

	s := scriptValues
	for _, pair := range [][2]string{
		{"Adlam", "Adlm"}, {"Ahom", "Ahom"}, {"Anatolian_Hieroglyphs", "Hluw"},
		{"Arabic", "Arab"}, {"Armenian", "Armn"}, {"Avestan", "Avst"},
		{"Balinese", "Bali"}, {"Bamum", "Bamu"}, {"Bassa_Vah", "Bass"},
		{"Batak", "Batk"}, {"Bengali", "Beng"}, {"Beria_Erfe", "Berf"},
		{"Bhaiksuki", "Bhks"}, {"Bopomofo", "Bopo"}, {"Brahmi", "Brah"},
		{"Braille", "Brai"}, {"Buginese", "Bugi"}, {"Buhid", "Buhd"},
		{"Canadian_Aboriginal", "Cans"}, {"Carian", "Cari"},
		{"Caucasian_Albanian", "Aghb"}, {"Chakma", "Cakm"}, {"Cham", "Cham"},
		{"Cherokee", "Cher"}, {"Chorasmian", "Chrs"}, {"Common", "Zyyy"},
		{"Coptic", "Copt"}, {"Cuneiform", "Xsux"}, {"Cypriot", "Cprt"},
		{"Cypro_Minoan", "Cpmn"}, {"Cyrillic", "Cyrl"}, {"Deseret", "Dsrt"},
		{"Devanagari", "Deva"}, {"Dives_Akuru", "Diak"}, {"Dogra", "Dogr"},
		{"Duployan", "Dupl"}, {"Egyptian_Hieroglyphs", "Egyp"},
		{"Elbasan", "Elba"}, {"Elymaic", "Elym"}, {"Ethiopic", "Ethi"},
		{"Garay", "Gara"}, {"Georgian", "Geor"}, {"Glagolitic", "Glag"},
		{"Gothic", "Goth"}, {"Grantha", "Gran"}, {"Greek", "Grek"},
		{"Gujarati", "Gujr"}, {"Gunjala_Gondi", "Gong"}, {"Gurmukhi", "Guru"},
		{"Gurung_Khema", "Gukh"}, {"Han", "Hani"}, {"Hangul", "Hang"},
		{"Hanifi_Rohingya", "Rohg"}, {"Hanunoo", "Hano"}, {"Hatran", "Hatr"},
		{"Hebrew", "Hebr"}, {"Hiragana", "Hira"}, {"Imperial_Aramaic", "Armi"},
		{"Inherited", "Zinh"}, {"Inscriptional_Pahlavi", "Phli"},
		{"Inscriptional_Parthian", "Prti"}, {"Javanese", "Java"},
		{"Kaithi", "Kthi"}, {"Kannada", "Knda"}, {"Katakana", "Kana"},
		{"Kawi", "Kawi"},
		{"Kayah_Li", "Kali"}, {"Kharoshthi", "Khar"},
		{"Khitan_Small_Script", "Kits"}, {"Khmer", "Khmr"}, {"Khojki", "Khoj"},
		{"Khudawadi", "Sind"}, {"Kirat_Rai", "Krai"}, {"Lao", "Laoo"},
		{"Latin", "Latn"}, {"Lepcha", "Lepc"}, {"Limbu", "Limb"},
		{"Linear_A", "Lina"}, {"Linear_B", "Linb"}, {"Lisu", "Lisu"},
		{"Lycian", "Lyci"}, {"Lydian", "Lydi"}, {"Mahajani", "Mahj"},
		{"Makasar", "Maka"}, {"Malayalam", "Mlym"}, {"Mandaic", "Mand"},
		{"Manichaean", "Mani"}, {"Marchen", "Marc"}, {"Masaram_Gondi", "Gonm"},
		{"Medefaidrin", "Medf"}, {"Meetei_Mayek", "Mtei"},
		{"Mende_Kikakui", "Mend"}, {"Meroitic_Cursive", "Merc"},
		{"Meroitic_Hieroglyphs", "Mero"}, {"Miao", "Plrd"}, {"Modi", "Modi"},
		{"Mongolian", "Mong"}, {"Mro", "Mroo"}, {"Multani", "Mult"},
		{"Myanmar", "Mymr"}, {"Nabataean", "Nbat"}, {"Nag_Mundari", "Nagm"},
		{"Nandinagari", "Nand"}, {"New_Tai_Lue", "Talu"}, {"Newa", "Newa"},
		{"Nko", "Nkoo"}, {"Nushu", "Nshu"}, {"Nyiakeng_Puachue_Hmong", "Hmnp"},
		{"Ogham", "Ogam"}, {"Ol_Chiki", "Olck"}, {"Ol_Onal", "Onao"},
		{"Old_Hungarian", "Hung"}, {"Old_Italic", "Ital"},
		{"Old_North_Arabian", "Narb"}, {"Old_Permic", "Perm"},
		{"Old_Persian", "Xpeo"}, {"Old_Sogdian", "Sogo"},
		{"Old_South_Arabian", "Sarb"}, {"Old_Turkic", "Orkh"},
		{"Old_Uyghur", "Ougr"}, {"Oriya", "Orya"}, {"Osage", "Osge"},
		{"Osmanya", "Osma"}, {"Pahawh_Hmong", "Hmng"}, {"Palmyrene", "Palm"},
		{"Pau_Cin_Hau", "Pauc"}, {"Phags_Pa", "Phag"}, {"Phoenician", "Phnx"},
		{"Psalter_Pahlavi", "Phlp"}, {"Rejang", "Rjng"}, {"Runic", "Runr"},
		{"Samaritan", "Samr"}, {"Saurashtra", "Saur"}, {"Sharada", "Shrd"},
		{"Shavian", "Shaw"}, {"Siddham", "Sidd"}, {"Sidetic", "Sidt"},
		{"SignWriting", "Sgnw"}, {"Sinhala", "Sinh"}, {"Sogdian", "Sogd"},
		{"Sora_Sompeng", "Sora"}, {"Soyombo", "Soyo"}, {"Sundanese", "Sund"},
		{"Sunuwar", "Sunu"}, {"Syloti_Nagri", "Sylo"}, {"Syriac", "Syrc"},
		{"Tagalog", "Tglg"}, {"Tagbanwa", "Tagb"}, {"Tai_Le", "Tale"},
		{"Tai_Tham", "Lana"}, {"Tai_Viet", "Tavt"}, {"Tai_Yo", "Tayo"},
		{"Takri", "Takr"}, {"Tamil", "Taml"}, {"Tangsa", "Tnsa"},
		{"Tangut", "Tang"}, {"Telugu", "Telu"}, {"Thaana", "Thaa"},
		{"Thai", "Thai"}, {"Tibetan", "Tibt"}, {"Tifinagh", "Tfng"},
		{"Tirhuta", "Tirh"}, {"Todhri", "Todr"}, {"Tolong_Siki", "Tols"},
		{"Toto", "Toto"}, {"Tulu_Tigalari", "Tutg"}, {"Ugaritic", "Ugar"},
		{"Unknown", "Zzzz"}, {"Vai", "Vaii"}, {"Vithkuqi", "Vith"},
		{"Wancho", "Wcho"}, {"Warang_Citi", "Wara"}, {"Yezidi", "Yezi"},
		{"Yi", "Yiii"}, {"Zanabazar_Square", "Zanb"},
	} {
		register(s, pair[0], pair[1])
	}
	// The extra aliases PropertyValueAliases.txt carries.
	register(s, "Coptic", "Qaac")
	register(s, "Inherited", "Qaai")
}

// ScriptNames lists every canonical Script value name, sorted.
func ScriptNames() []string { return canonicalNames(scriptValues) }

// BinaryPropertyNames lists every canonical binary property name, sorted.
func BinaryPropertyNames() []string { return canonicalNames(binaryProperties) }

// GeneralCategoryNames lists every canonical General_Category value, sorted.
func GeneralCategoryNames() []string { return canonicalNames(generalCategoryValues) }

func canonicalNames(m map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range m {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

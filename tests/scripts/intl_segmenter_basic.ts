// expect: ok
// Intl.Segmenter (#210): default-locale grapheme/word/sentence segmentation,
// Segments iteration + containing(), resolvedOptions, supportedLocalesOf,
// and the Intl namespace object itself.

const results: string[] = [];
function check(name: string, cond: boolean) {
  if (!cond) results.push(name);
}

check("typeof Intl", typeof Intl === "object");
check("Intl toStringTag", Object.prototype.toString.call(Intl) === "[object Intl]");
check("Segmenter is function", typeof Intl.Segmenter === "function");

// Graphemes: combining marks, ZWJ emoji sequences and flags are single
// segments; indices are UTF-16 code units.
const graphemes: Intl.Segmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });
const text = "éa👨‍👩‍👧‍👦🇵🇱b";
const gs: Intl.SegmentData[] = [...graphemes.segment(text)];
check("grapheme count", gs.length === 5);
check("grapheme[0]", gs[0].segment === "é" && gs[0].index === 0);
check("grapheme[1]", gs[1].segment === "a" && gs[1].index === 2);
check("grapheme[2] family", gs[2].segment === "👨‍👩‍👧‍👦" && gs[2].index === 3);
check("grapheme[3] flag", gs[3].segment === "🇵🇱" && gs[3].index === 14);
check("grapheme[4]", gs[4].segment === "b" && gs[4].index === 18);
check("grapheme isWordLike absent", !("isWordLike" in gs[0]));
check("grapheme input", gs[0].input === text);
check("grapheme roundtrip", gs.map((g) => g.segment).join("") === text);

// Lone surrogates are their own segments; a pair built by concatenation is
// the same JS string as the literal and segments identically.
const lone = [...graphemes.segment("\ud800 \udc00")];
check("lone surrogates", lone.length === 3 && lone[0].segment === "\ud800" && lone[2].segment === "\udc00" && lone[2].index === 2);
const joined = "\ud800" + "\udc00";
const pairSegs = [...graphemes.segment(joined + "x")];
check("concatenated pair", pairSegs.length === 2 && pairSegs[0].segment === "𐀀" && pairSegs[1].index === 2);
check("containing trail surrogate", graphemes.segment(joined).containing(1)!.segment === joined);

// Words: isWordLike marks letter/number runs; punctuation and spaces are not.
const words = new Intl.Segmenter("en", { granularity: "word" });
const ws = [...words.segment("Hello, wörld! It's 3.14")];
check("word segments", ws.map((w) => w.segment).join("|") === "Hello|,| |wörld|!| |It's| |3.14");
check("word isWordLike", ws.map((w) => (w.isWordLike ? "1" : "0")).join("") === "100100101");
check("word index", ws[3].index === 7 && ws[3].input === "Hello, wörld! It's 3.14");

// Sentences.
const sentences = new Intl.Segmenter("en", { granularity: "sentence" });
const ss = [...sentences.segment("Hello world! How are you? Fine.")];
check("sentence segments", ss.map((s) => s.segment).join("|") === "Hello world! |How are you? |Fine.");

// containing(): by UTF-16 index, undefined when out of range; the Segments
// object can be iterated repeatedly and independently of containing().
const segs: Intl.Segments = words.segment("ab cd");
check("containing(1)", segs.containing(1)!.segment === "ab" && segs.containing(1)!.index === 0);
check("containing(2)", segs.containing(2)!.segment === " ");
check("containing(4)", segs.containing(4)!.segment === "cd" && segs.containing(4)!.index === 3);
check("containing(-1)", segs.containing(-1) === undefined);
check("containing(5)", segs.containing(5) === undefined);
check("containing default", segs.containing()!.index === 0);
check("re-iterable", [...segs].length === 3 && [...segs].length === 3);

// Iterator protocol details.
const it = segs[Symbol.iterator]();
check("iterator toStringTag", Object.prototype.toString.call(it) === "[object Segmenter String Iterator]");
check("iterator next", it.next().value.segment === "ab" && it.next().value.segment === " " && it.next().value.segment === "cd" && it.next().done === true);

// Options and locales.
check("default granularity", new Intl.Segmenter().resolvedOptions().granularity === "grapheme");
check("locale canonicalized", new Intl.Segmenter("en-us").resolvedOptions().locale === "en-US");
check("unsupported falls through", new Intl.Segmenter(["xyz", "de"]).resolvedOptions().locale === "de");
check("extensions dropped", new Intl.Segmenter("pl-PL-u-nu-latn").resolvedOptions().locale === "pl-PL");
check("supportedLocalesOf", Intl.Segmenter.supportedLocalesOf(["sr-Thai-RS", "zxx", "de"]).join(",") === "sr-Thai-RS,de");
check("getCanonicalLocales", Intl.getCanonicalLocales(["EN-us", "en-US", "iw", "zh-hans-cn-u-nu-latn-ca-gregory"]).join(",") === "en-US,he,zh-Hans-CN-u-ca-gregory-nu-latn");
check("prototype toStringTag", Object.prototype.toString.call(words) === "[object Intl.Segmenter]");

let threw = "";
try { (Intl.Segmenter as any)(); } catch (e) { threw = (e as Error).constructor.name; }
check("requires new", threw === "TypeError");
threw = "";
try { new Intl.Segmenter("en-GB-oed"); } catch (e) { threw = (e as Error).constructor.name; }
check("invalid tag RangeError", threw === "RangeError");
threw = "";
try { new Intl.Segmenter(undefined, { granularity: "line" }); } catch (e) { threw = (e as Error).constructor.name; }
check("invalid granularity RangeError", threw === "RangeError");
threw = "";
try { Intl.Segmenter.prototype.segment.call({}, "x"); } catch (e) { threw = (e as Error).constructor.name; }
check("branding TypeError", threw === "TypeError");

results.length === 0 ? "ok" : "failed: " + results.join(", ");

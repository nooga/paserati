// expect: true

const asyncArrayPromise = Array.fromAsync([1, 2, 3]);

const bytes: Uint8Array = Uint8Array.fromBase64("AQI=");
const bytesHex: string = bytes.toHex();
const base64: string = bytes.toBase64();
const writeResult = new Uint8Array(2).setFromHex("0a0b");

const nums: Uint16Array = new Uint16Array([1, 2, 3]);
const last: number | undefined = nums.findLast((value) => value > 1);
const lastIndex: number = nums.findLastIndex((value) => value === 2);
const reversed: Uint16Array = nums.toReversed();
const sorted: Uint16Array = nums.toSorted((a, b) => b - a);
const replaced: Uint16Array = nums.with(0, 9);

const map: Map<string, number> = new Map();
const inserted: number = map.getOrInsert("a", 1);
const computed: number = map.getOrInsertComputed("bb", (key) => key.length);

const weakKey = {};
const weakMap: WeakMap<object, number> = new WeakMap();
const weakInserted: number = weakMap.getOrInsert(weakKey, 3);
const weakComputed: number = weakMap.getOrInsertComputed({}, () => 4);

const iteratorReturn = Iterator.from([1]).map((value) => value + 1).return();

asyncArrayPromise !== undefined &&
	bytesHex === "0102" &&
	base64 === "AQI=" &&
	writeResult.written === 2 &&
	last === 3 &&
	lastIndex === 1 &&
	reversed[0] === 3 &&
	sorted[0] === 3 &&
	replaced[0] === 9 &&
	inserted === 1 &&
	computed === 2 &&
	weakInserted === 3 &&
	weakComputed === 4 &&
	iteratorReturn.done === true;

// expect: 5043d77c2a81ce31619f3e77b7a929a22340fc53
/*
 * A JavaScript implementation of the Secure Hash Algorithm, SHA-1, as defined
 * in FIPS PUB 180-1
 * Version 2.1a Copyright Paul Johnston 2000 - 2002.
 * Other contributors: Greg Holt, Andrew Kepert, Ydnar, Lostinet
 * Distributed under the BSD License
 * See http://pajhome.org.uk/crypt/md5 for details.
 */

/*
 * Configurable letiables. You may need to tweak these to be compatible with
 * the server-side, but the defaults work in most cases.
 */
let hexcase = 0; /* hex output format. 0 - lowercase; 1 - uppercase        */
let b64pad = ""; /* base-64 pad character. "=" for strict RFC compliance   */
let chrsz = 8; /* bits per input character. 8 - ASCII; 16 - Unicode      */

/*
 * These are the functions you'll usually want to call
 * They take string arguments and return either hex or base-64 encoded strings
 */
function hex_sha1(s: string): string {
  console.log("hex_sha1 called with string length:", s.length);
  //console.log("hex_sha1: About to call str2binb with s =", s);
  let binb = str2binb(s);
  console.log("str2binb result length:", binb.length);
  let core = core_sha1(binb, s.length * chrsz);
  console.log("core_sha1 result:", core);
  let result = binb2hex(core);
  console.log("binb2hex result:", result);
  return result;
}
function b64_sha1(s: string): string {
  return binb2b64(core_sha1(str2binb(s), s.length * chrsz));
}
function str_sha1(s: string): string {
  return binb2str(core_sha1(str2binb(s), s.length * chrsz));
}
function hex_hmac_sha1(key: string, data: string): string {
  return binb2hex(core_hmac_sha1(key, data));
}
function b64_hmac_sha1(key: string, data: string): string {
  return binb2b64(core_hmac_sha1(key, data));
}
function str_hmac_sha1(key: string, data: string): string {
  return binb2str(core_hmac_sha1(key, data));
}

/*
 * Perform a simple self-test to see if the VM is working
 */
function sha1_vm_test(): boolean {
  return hex_sha1("abc") == "a9993e364706816aba3e25717850c26c9cd0d89d";
}

/*
 * Calculate the SHA-1 of an array of big-endian words, and a bit length
 */
function core_sha1(x: number[], len: number): number[] {
  console.log("core_sha1 called with x.length:", x.length, "len:", len);
  /* append padding */
  x[len >> 5] |= 0x80 << (24 - (len % 32));
  x[(((len + 64) >> 9) << 4) + 15] = len;

  let w = Array(80);
  let a = 1732584193;
  let b = -271733879;
  let c = -1732584194;
  let d = 271733878;
  let e = -1009589776;

  console.log("Initial values: a =", a, "b =", b, "c =", c, "d =", d, "e =", e);

  for (let i = 0; i < x.length; i += 16) {
    console.log("Processing block", i / 16, "i =", i);
    let olda = a;
    let oldb = b;
    let oldc = c;
    let oldd = d;
    let olde = e;

    for (let j = 0; j < 80; j++) {
      if (j < 16) {
        w[j] = x[i + j];
      } else {
        w[j] = rol(w[j - 3] ^ w[j - 8] ^ w[j - 14] ^ w[j - 16], 1);
      }
      let t = safe_add(
        safe_add(rol(a, 5), sha1_ft(j, b, c, d)),
        safe_add(safe_add(e, w[j]), sha1_kt(j))
      );
      e = d;
      d = c;
      c = rol(b, 30);
      b = a;
      a = t;

      if (j < 5 || j % 20 === 0) {
        console.log(
          "Round",
          j,
          ": a =",
          a,
          "b =",
          b,
          "c =",
          c,
          "d =",
          d,
          "e =",
          e
        );
      }
    }

    a = safe_add(a, olda);
    b = safe_add(b, oldb);
    c = safe_add(c, oldc);
    d = safe_add(d, oldd);
    e = safe_add(e, olde);

    console.log(
      "Block",
      i / 16,
      "final: a =",
      a,
      "b =",
      b,
      "c =",
      c,
      "d =",
      d,
      "e =",
      e
    );
  }
  let result = Array(a, b, c, d, e);
  console.log("core_sha1 returning:", result);
  return result;
}

/*
 * Perform the appropriate triplet combination function for the current
 * iteration
 */
function sha1_ft(t: number, b: number, c: number, d: number): number {
  if (t < 20) {
    return (b & c) | (~b & d);
  }
  if (t < 40) {
    return b ^ c ^ d;
  }
  if (t < 60) {
    return (b & c) | (b & d) | (c & d);
  }
  return b ^ c ^ d;
}

/*
 * Determine the appropriate additive constant for the current iteration
 */
function sha1_kt(t: number): number {
  return t < 20
    ? 1518500249
    : t < 40
    ? 1859775393
    : t < 60
    ? -1894007588
    : -899497514;
}

/*
 * Calculate the HMAC-SHA1 of a key and some data
 */
function core_hmac_sha1(key: string, data: string): number[] {
  let bkey = str2binb(key);
  if (bkey.length > 16) {
    bkey = core_sha1(bkey, key.length * chrsz);
  }

  let ipad = Array(16);
  let opad = Array(16);
  for (let i = 0; i < 16; i++) {
    ipad[i] = bkey[i] ^ 0x36363636;
    opad[i] = bkey[i] ^ 0x5c5c5c5c;
  }

  let hash = core_sha1(ipad.concat(str2binb(data)), 512 + data.length * chrsz);
  return core_sha1(opad.concat(hash), 512 + 160);
}

/*
 * Add integers, wrapping at 2^32. This uses 16-bit operations internally
 * to work around bugs in some JS interpreters.
 */
function safe_add(x: number, y: number): number {
  let lsw = (x & 0xffff) + (y & 0xffff);
  let msw = (x >> 16) + (y >> 16) + (lsw >> 16);
  return (msw << 16) | (lsw & 0xffff);
}

/*
 * Bitwise rotate a 32-bit number to the left.
 */
function rol(num: number, cnt: number): number {
  return (num << cnt) | (num >>> (32 - cnt));
}

/*
 * Convert an 8-bit or 16-bit string to an array of big-endian words
 * In 8-bit function, characters >255 have their hi-byte silently ignored.
 */
function str2binb(str: string): number[] {
  //console.log("str2binb ENTRY: received parameter str =", str);
  console.log("str2binb ENTRY: str === undefined =", str === undefined);
  console.log("str2binb ENTRY: str === null =", str === null);
  console.log("str2binb ENTRY: About to access str.length");

  console.log("str2binb called with string length:", str.length);
  console.log("str2binb: About to call Array()");
  let bin = Array();
  console.log("str2binb: Array() returned, bin =", bin);

  console.log("str2binb: About to access chrsz, chrsz =", chrsz);
  let mask = (1 << chrsz) - 1;
  console.log("mask:", mask, "chrsz:", chrsz);
  console.log(
    "str2binb: About to enter for loop, str.length * chrsz =",
    str.length * chrsz
  );

  for (let i = 0; i < str.length * chrsz; i += chrsz) {
    console.log("str2binb: Loop iteration i =", i);
    let charCode = str.charCodeAt(i / chrsz);
    console.log("str2binb: charCode =", charCode);
    let index = i >> 5;
    let shift = 32 - chrsz - (i % 32);
    let value = (charCode & mask) << shift;

    console.log("str2binb: bin[index] before check =", bin[index]);
    if (bin[index] === undefined) {
      console.log("str2binb: Setting bin[index] to 0");
      bin[index] = 0;
    }
    console.log("str2binb: About to do bin[index] |= value");
    bin[index] |= value;

    if (i < 50) {
      // Log first few iterations
      console.log(
        "i =",
        i,
        "charCode =",
        charCode,
        "index =",
        index,
        "shift =",
        shift,
        "value =",
        value,
        "bin[index] =",
        bin[index]
      );
    }

    if (i > 100) {
      console.log("str2binb: Breaking loop early for debugging after i =", i);
      break;
    }
  }
  console.log(
    "str2binb result length:",
    bin.length,
    "first few elements:",
    bin
  );
  return bin;
}

/*
 * Convert an array of big-endian words to a string
 */
function binb2str(bin: number[]): string {
  let str = "";
  let mask = (1 << chrsz) - 1;
  for (let i = 0; i < bin.length * 32; i += chrsz) {
    str += String.fromCharCode(
      (bin[i >> 5] >>> (32 - chrsz - (i % 32))) & mask
    );
  }
  return str;
}

/*
 * Convert an array of big-endian words to a hex string.
 */
function binb2hex(binarray: number[]): string {
  console.log(
    "binb2hex called with array length:",
    binarray.length,
    "array:",
    binarray
  );
  let hex_tab = hexcase ? "0123456789ABCDEF" : "0123456789abcdef";
  let str = "";
  for (let i = 0; i < binarray.length * 4; i++) {
    let index = i >> 2;
    let shift1 = (3 - (i % 4)) * 8 + 4;
    let shift2 = (3 - (i % 4)) * 8;
    let val1 = (binarray[index] >> shift1) & 0xf;
    let val2 = (binarray[index] >> shift2) & 0xf;

    str += hex_tab.charAt(val1) + hex_tab.charAt(val2);

    if (i < 10) {
      // Log first few iterations
      console.log(
        "i =",
        i,
        "index =",
        index,
        "shift1 =",
        shift1,
        "shift2 =",
        shift2,
        "val1 =",
        val1,
        "val2 =",
        val2,
        "chars:",
        hex_tab.charAt(val1),
        hex_tab.charAt(val2)
      );
    }
  }
  console.log("binb2hex result:", str);
  return str;
}

/*
 * Convert an array of big-endian words to a base-64 string
 */
function binb2b64(binarray: number[]): string {
  let tab = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  let str = "";
  for (let i = 0; i < binarray.length * 4; i += 3) {
    let triplet =
      (((binarray[i >> 2] >> (8 * (3 - (i % 4)))) & 0xff) << 16) |
      (((binarray[(i + 1) >> 2] >> (8 * (3 - ((i + 1) % 4)))) & 0xff) << 8) |
      ((binarray[(i + 2) >> 2] >> (8 * (3 - ((i + 2) % 4)))) & 0xff);
    for (let j = 0; j < 4; j++) {
      if (i * 8 + j * 6 > binarray.length * 32) {
        str += b64pad;
      } else {
        str += tab.charAt((triplet >> (6 * (3 - j))) & 0x3f);
      }
    }
  }
  return str;
}

let plainText =
  "Two households, both alike in dignity,\n\
In fair Verona, where we lay our scene,\n\
From ancient grudge break to new mutiny,\n\
Where civil blood makes civil hands unclean.\n\
From forth the fatal loins of these two foes\n\
A pair of star-cross'd lovers take their life;\n\
Whole misadventured piteous overthrows\n\
Do with their death bury their parents' strife.\n\
The fearful passage of their death-mark'd love,\n\
And the continuance of their parents' rage,\n\
Which, but their children's end, nought could remove,\n\
Is now the two hours' traffic of our stage;\n\
The which if you with patient ears attend,\n\
What here shall miss, our toil shall strive to mend.";

console.log("Starting execution...");
console.log("Initial plainText length:", plainText.length);

for (let i = 0; i < 4; i++) {
  plainText += plainText;
  console.log("After iteration", i, "plainText length:", plainText.length);
}

console.log("Final plainText length:", plainText.length);
console.log("Calling hex_sha1...");

let sha1Output = hex_sha1(plainText);
console.log("Final sha1Output:", sha1Output);
sha1Output;

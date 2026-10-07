// expect: SS FI ʼN 2 ας STRASSE d800
// #611: full Unicode case mappings (SpecialCasing.txt, Final_Sigma)
["ß".toUpperCase(), "ﬁ".toUpperCase(), "ŉ".toUpperCase(), "İ".toLowerCase().length,
 "ΑΣ".toLowerCase(), "Straße".toLocaleUpperCase(),
 "\ud800x".toUpperCase().charCodeAt(0).toString(16)].join(" ");

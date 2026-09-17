// A chained assignment expression statement (`a.b = c.d = ... = value`)
// used to leak its intermediate object-reference registers into whatever
// came after it in the same function scope: each link's own base register
// stayed live for the entire nested RHS compile (a plain recursive call per
// link), so a chain N levels deep held N registers simultaneously just to
// move them back into a register moments later - real register pressure
// with nothing computed from it in between. A big block of local
// declarations placed after such a chain (this shape mirrors real
// @babel/types generated code, which does exactly this) could then run the
// function's 255-register budget dry and fail to compile. See #470.
// expect: true

function repro(): boolean {
  let exportsObj: Record<string, number> = {};

  // 180-link chained property assignment.
  exportsObj.n0 = exportsObj.n1 = exportsObj.n2 = exportsObj.n3 = exportsObj.n4 =
  exportsObj.n5 = exportsObj.n6 = exportsObj.n7 = exportsObj.n8 = exportsObj.n9 =
  exportsObj.n10 = exportsObj.n11 = exportsObj.n12 = exportsObj.n13 = exportsObj.n14 =
  exportsObj.n15 = exportsObj.n16 = exportsObj.n17 = exportsObj.n18 = exportsObj.n19 =
  exportsObj.n20 = exportsObj.n21 = exportsObj.n22 = exportsObj.n23 = exportsObj.n24 =
  exportsObj.n25 = exportsObj.n26 = exportsObj.n27 = exportsObj.n28 = exportsObj.n29 =
  exportsObj.n30 = exportsObj.n31 = exportsObj.n32 = exportsObj.n33 = exportsObj.n34 =
  exportsObj.n35 = exportsObj.n36 = exportsObj.n37 = exportsObj.n38 = exportsObj.n39 =
  exportsObj.n40 = exportsObj.n41 = exportsObj.n42 = exportsObj.n43 = exportsObj.n44 =
  exportsObj.n45 = exportsObj.n46 = exportsObj.n47 = exportsObj.n48 = exportsObj.n49 =
  exportsObj.n50 = exportsObj.n51 = exportsObj.n52 = exportsObj.n53 = exportsObj.n54 =
  exportsObj.n55 = exportsObj.n56 = exportsObj.n57 = exportsObj.n58 = exportsObj.n59 =
  exportsObj.n60 = exportsObj.n61 = exportsObj.n62 = exportsObj.n63 = exportsObj.n64 =
  exportsObj.n65 = exportsObj.n66 = exportsObj.n67 = exportsObj.n68 = exportsObj.n69 =
  exportsObj.n70 = exportsObj.n71 = exportsObj.n72 = exportsObj.n73 = exportsObj.n74 =
  exportsObj.n75 = exportsObj.n76 = exportsObj.n77 = exportsObj.n78 = exportsObj.n79 =
  exportsObj.n80 = exportsObj.n81 = exportsObj.n82 = exportsObj.n83 = exportsObj.n84 =
  exportsObj.n85 = exportsObj.n86 = exportsObj.n87 = exportsObj.n88 = exportsObj.n89 =
  exportsObj.n90 = exportsObj.n91 = exportsObj.n92 = exportsObj.n93 = exportsObj.n94 =
  exportsObj.n95 = exportsObj.n96 = exportsObj.n97 = exportsObj.n98 = exportsObj.n99 =
  exportsObj.n100 = exportsObj.n101 = exportsObj.n102 = exportsObj.n103 =
  exportsObj.n104 = exportsObj.n105 = exportsObj.n106 = exportsObj.n107 =
  exportsObj.n108 = exportsObj.n109 = exportsObj.n110 = exportsObj.n111 =
  exportsObj.n112 = exportsObj.n113 = exportsObj.n114 = exportsObj.n115 =
  exportsObj.n116 = exportsObj.n117 = exportsObj.n118 = exportsObj.n119 =
  exportsObj.n120 = exportsObj.n121 = exportsObj.n122 = exportsObj.n123 =
  exportsObj.n124 = exportsObj.n125 = exportsObj.n126 = exportsObj.n127 =
  exportsObj.n128 = exportsObj.n129 = exportsObj.n130 = exportsObj.n131 =
  exportsObj.n132 = exportsObj.n133 = exportsObj.n134 = exportsObj.n135 =
  exportsObj.n136 = exportsObj.n137 = exportsObj.n138 = exportsObj.n139 =
  exportsObj.n140 = exportsObj.n141 = exportsObj.n142 = exportsObj.n143 =
  exportsObj.n144 = exportsObj.n145 = exportsObj.n146 = exportsObj.n147 =
  exportsObj.n148 = exportsObj.n149 = exportsObj.n150 = exportsObj.n151 =
  exportsObj.n152 = exportsObj.n153 = exportsObj.n154 = exportsObj.n155 =
  exportsObj.n156 = exportsObj.n157 = exportsObj.n158 = exportsObj.n159 =
  exportsObj.n160 = exportsObj.n161 = exportsObj.n162 = exportsObj.n163 =
  exportsObj.n164 = exportsObj.n165 = exportsObj.n166 = exportsObj.n167 =
  exportsObj.n168 = exportsObj.n169 = exportsObj.n170 = exportsObj.n171 =
  exportsObj.n172 = exportsObj.n173 = exportsObj.n174 = exportsObj.n175 =
  exportsObj.n176 = exportsObj.n177 = exportsObj.n178 = exportsObj.n179
    = 1;

  // 150-declarator const block right after it, in the same scope.
  const
  m0 = 0, m1 = 1, m2 = 2, m3 = 3, m4 = 4, m5 = 5, m6 = 6, m7 = 7, m8 = 8, m9 = 9,
  m10 = 10, m11 = 11, m12 = 12, m13 = 13, m14 = 14, m15 = 15, m16 = 16, m17 = 17,
  m18 = 18, m19 = 19, m20 = 20, m21 = 21, m22 = 22, m23 = 23, m24 = 24, m25 = 25,
  m26 = 26, m27 = 27, m28 = 28, m29 = 29, m30 = 30, m31 = 31, m32 = 32, m33 = 33,
  m34 = 34, m35 = 35, m36 = 36, m37 = 37, m38 = 38, m39 = 39, m40 = 40, m41 = 41,
  m42 = 42, m43 = 43, m44 = 44, m45 = 45, m46 = 46, m47 = 47, m48 = 48, m49 = 49,
  m50 = 50, m51 = 51, m52 = 52, m53 = 53, m54 = 54, m55 = 55, m56 = 56, m57 = 57,
  m58 = 58, m59 = 59, m60 = 60, m61 = 61, m62 = 62, m63 = 63, m64 = 64, m65 = 65,
  m66 = 66, m67 = 67, m68 = 68, m69 = 69, m70 = 70, m71 = 71, m72 = 72, m73 = 73,
  m74 = 74, m75 = 75, m76 = 76, m77 = 77, m78 = 78, m79 = 79, m80 = 80, m81 = 81,
  m82 = 82, m83 = 83, m84 = 84, m85 = 85, m86 = 86, m87 = 87, m88 = 88, m89 = 89,
  m90 = 90, m91 = 91, m92 = 92, m93 = 93, m94 = 94, m95 = 95, m96 = 96, m97 = 97,
  m98 = 98, m99 = 99, m100 = 100, m101 = 101, m102 = 102, m103 = 103, m104 = 104,
  m105 = 105, m106 = 106, m107 = 107, m108 = 108, m109 = 109, m110 = 110, m111 = 111,
  m112 = 112, m113 = 113, m114 = 114, m115 = 115, m116 = 116, m117 = 117, m118 = 118,
  m119 = 119, m120 = 120, m121 = 121, m122 = 122, m123 = 123, m124 = 124, m125 = 125,
  m126 = 126, m127 = 127, m128 = 128, m129 = 129, m130 = 130, m131 = 131, m132 = 132,
  m133 = 133, m134 = 134, m135 = 135, m136 = 136, m137 = 137, m138 = 138, m139 = 139,
  m140 = 140, m141 = 141, m142 = 142, m143 = 143, m144 = 144, m145 = 145, m146 = 146,
  m147 = 147, m148 = 148, m149 = 149;

  let chainOk = true;
  for (let i = 0; i < 180; i++) {
    if (exportsObj["n" + i] !== 1) chainOk = false;
  }

  return chainOk && m0 === 0 && m149 === 149;
}

repro();
